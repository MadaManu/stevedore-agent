package apache

import "testing"

func TestResolvePathConfig_Defaults(t *testing.T) {
	cfg, err := resolvePathConfig(map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.pathPrefix != "/" {
		t.Fatalf("expected root path prefix, got %q", cfg.pathPrefix)
	}
	if !cfg.stripPathPrefix {
		t.Fatal("expected stripPathPrefix default to true")
	}
}

func TestResolvePathConfig_RejectsInvalidPathType(t *testing.T) {
	if _, err := resolvePathConfig(map[string]interface{}{"path": true}); err == nil {
		t.Fatal("expected invalid path type to fail")
	}
}

func TestResolvePathConfig_RejectsInvalidStripPathType(t *testing.T) {
	if _, err := resolvePathConfig(map[string]interface{}{"stripPathPrefix": "true"}); err == nil {
		t.Fatal("expected invalid stripPathPrefix type to fail")
	}
}

func TestResolvePathConfig_NormalizesPath(t *testing.T) {
	cfg, err := resolvePathConfig(map[string]interface{}{"path": "/hello/"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.pathPrefix != "/hello" {
		t.Fatalf("expected normalized path '/hello', got %q", cfg.pathPrefix)
	}
}

func TestNormalizePathPrefix_RejectsMissingSlash(t *testing.T) {
	if _, err := normalizePathPrefix("hello"); err == nil {
		t.Fatal("expected path without leading slash to fail")
	}
}

func TestNormalizePathPrefix_RejectsQueryAndFragment(t *testing.T) {
	if _, err := normalizePathPrefix("/hello?x=1"); err == nil {
		t.Fatal("expected path containing query to fail")
	}
	if _, err := normalizePathPrefix("/hello#anchor"); err == nil {
		t.Fatal("expected path containing fragment to fail")
	}
}

func TestBuildRouteConfig_DefaultRoot(t *testing.T) {
	route := buildRouteConfig(pathConfig{pathPrefix: "/", stripPathPrefix: true}, 8081)
	if route.proxyPath != "/" {
		t.Fatalf("expected proxyPath '/', got %q", route.proxyPath)
	}
	if route.proxyTarget != "http://127.0.0.1:8081/" {
		t.Fatalf("unexpected proxyTarget: %q", route.proxyTarget)
	}
	if route.hasCustomPathPrefix {
		t.Fatal("expected hasCustomPathPrefix to be false")
	}
}

func TestBuildRouteConfig_PathStripsPrefixByDefault(t *testing.T) {
	route := buildRouteConfig(pathConfig{pathPrefix: "/hello", stripPathPrefix: true}, 8081)
	if route.proxyPath != "/hello/" {
		t.Fatalf("expected proxyPath '/hello/', got %q", route.proxyPath)
	}
	if route.proxyTarget != "http://127.0.0.1:8081/" {
		t.Fatalf("unexpected proxyTarget: %q", route.proxyTarget)
	}
	if route.pathPrefixRegex != "/hello" {
		t.Fatalf("unexpected pathPrefixRegex: %q", route.pathPrefixRegex)
	}
}

func TestBuildRouteConfig_PathKeepsPrefixWhenRequested(t *testing.T) {
	route := buildRouteConfig(pathConfig{pathPrefix: "/hello", stripPathPrefix: false}, 8081)
	if route.proxyTarget != "http://127.0.0.1:8081/hello/" {
		t.Fatalf("unexpected proxyTarget: %q", route.proxyTarget)
	}
}
