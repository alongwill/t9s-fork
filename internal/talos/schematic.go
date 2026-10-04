package talos

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultFactoryURL is assumed on nodes older than 1.14, which do not publish
// the factory URL.
const DefaultFactoryURL = "https://factory.talos.dev"

// SchematicType is the resource Talos >= 1.14 publishes with the schematic.
const SchematicType = "ImageFactorySchematics.runtime.talos.dev"

// SchematicInfo says which Image Factory schematic a node was built from.
type SchematicInfo struct {
	ID      string
	Flavor  string // empty on older nodes
	APIURL  string
	FromRes bool // read from the resource (false: the extension-list fallback)
}

var schematicIDRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ValidSchematicID reports whether id is a 64-hex-character schematic ID.
func ValidSchematicID(id string) bool { return schematicIDRe.MatchString(id) }

// SchematicURL builds <apiURL>/schematics/<id>. The ID is validated first so
// nothing else can end up in the path.
func SchematicURL(apiURL, id string) (string, error) {
	if !ValidSchematicID(id) {
		return "", fmt.Errorf("invalid schematic ID %q (want 64 hex characters)", id)
	}
	if apiURL == "" {
		apiURL = DefaultFactoryURL
	}
	u, err := url.Parse(apiURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", fmt.Errorf("invalid factory URL %q", apiURL)
	}
	return strings.TrimRight(u.String(), "/") + "/schematics/" + id, nil
}

// GetSchematicInfo reads the schematic from the ImageFactorySchematic
// resource, or, on nodes without it, from the extension named "schematic"
// whose version is the schematic ID (the factory is then assumed).
func (c *Client) GetSchematicInfo(ctx context.Context, node string) (SchematicInfo, error) {
	y, err := c.GetResourceYAML(ctx, node, "runtime", SchematicType, "image-factory-schematic")
	if err == nil {
		if info, ok := parseSchematicResource(y); ok {
			return info, nil
		}
	}
	exts, xerr := c.GetExtensions(ctx, node)
	if xerr != nil {
		if err != nil {
			return SchematicInfo{}, err
		}
		return SchematicInfo{}, xerr
	}
	if info, ok := SchematicFromExtensions(exts); ok {
		return info, nil
	}
	return SchematicInfo{}, errors.New("no schematic found on this node")
}

func parseSchematicResource(y string) (SchematicInfo, bool) {
	var doc struct {
		Spec struct {
			ID     string `yaml:"schematicId"`
			Flavor string `yaml:"flavor"`
			APIURL string `yaml:"apiUrl"`
		} `yaml:"spec"`
	}
	if yaml.Unmarshal([]byte(y), &doc) != nil || doc.Spec.ID == "" {
		return SchematicInfo{}, false
	}
	api := doc.Spec.APIURL
	if api == "" {
		api = DefaultFactoryURL
	}
	return SchematicInfo{ID: doc.Spec.ID, Flavor: doc.Spec.Flavor, APIURL: api, FromRes: true}, true
}

// SchematicFromExtensions is the older-node fallback.
func SchematicFromExtensions(exts []Extension) (SchematicInfo, bool) {
	for _, e := range exts {
		if e.Name == "schematic" && e.Version != "" {
			return SchematicInfo{ID: e.Version, APIURL: DefaultFactoryURL}, true
		}
	}
	return SchematicInfo{}, false
}

// ErrFactoryAuth means the factory answered 401 or 403.
var ErrFactoryAuth = errors.New("the factory needs authentication")

var (
	schematicMu    sync.Mutex
	schematicCache = map[string]string{}
	// SchematicTimeout bounds one factory request.
	SchematicTimeout = 10 * time.Second
)

// FetchSchematicYAML GETs the schematic from the Image Factory
// (Accept: application/yaml). A read, so it is allowed in read-only mode.
// Results are cached per apiURL+id for the session.
func FetchSchematicYAML(ctx context.Context, apiURL, id string) (string, error) {
	full, err := SchematicURL(apiURL, id)
	if err != nil {
		return "", err
	}
	schematicMu.Lock()
	cached, ok := schematicCache[full]
	schematicMu.Unlock()
	if ok {
		return cached, nil
	}
	ctx, cancel := context.WithTimeout(ctx, SchematicTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/yaml")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return "", ErrFactoryAuth
	case resp.StatusCode != http.StatusOK:
		return "", fmt.Errorf("factory answered %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	out := string(body)
	schematicMu.Lock()
	schematicCache[full] = out
	schematicMu.Unlock()
	return out, nil
}
