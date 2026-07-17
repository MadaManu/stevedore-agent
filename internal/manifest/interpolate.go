package manifest

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// SecretResolver resolves ${provider:path} placeholders at execution time.
type SecretResolver interface {
	Resolve(provider, path string) (string, error)
}

func resolveAllApplications(apps []Application, secretResolver SecretResolver) error {
	if len(apps) == 0 {
		return nil
	}

	maxIterations := len(apps)*4 + 8
	for i := 0; i < maxIterations; i++ {
		lookup, err := buildAppLookup(apps)
		if err != nil {
			return err
		}

		changed := false
		for ai := range apps {
			appChanged, err := resolveApplicationPlaceholders(&apps[ai], lookup, secretResolver)
			if err != nil {
				return fmt.Errorf("%s: %w", apps[ai].SourcePath, err)
			}
			changed = changed || appChanged
		}

		if !changed {
			break
		}
	}

	for i := range apps {
		if hasPlaceholders(apps[i]) {
			return fmt.Errorf("%s: unresolved placeholder(s) remain after interpolation", apps[i].SourcePath)
		}
	}

	return nil
}

func buildAppLookup(apps []Application) (map[string]any, error) {
	lookup := make(map[string]any, len(apps))
	for _, app := range apps {
		b, err := yaml.Marshal(app)
		if err != nil {
			return nil, err
		}
		var m map[string]any
		if err := yaml.Unmarshal(b, &m); err != nil {
			return nil, err
		}
		lookup[app.Metadata.Name] = m
	}
	return lookup, nil
}

func resolveApplicationPlaceholders(app *Application, lookup map[string]any, secretResolver SecretResolver) (bool, error) {
	v := reflect.ValueOf(app)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return false, nil
	}
	return resolveValue(v.Elem(), lookup, secretResolver)
}

func resolveValue(v reflect.Value, lookup map[string]any, secretResolver SecretResolver) (bool, error) {
	if !v.IsValid() {
		return false, nil
	}

	switch v.Kind() {
	case reflect.String:
		resolved, changed, err := resolveString(v.String(), lookup, secretResolver)
		if err != nil {
			return false, err
		}
		if changed {
			v.SetString(resolved)
		}
		return changed, nil
	case reflect.Ptr:
		if v.IsNil() {
			return false, nil
		}
		return resolveValue(v.Elem(), lookup, secretResolver)
	case reflect.Struct:
		changed := false
		for i := 0; i < v.NumField(); i++ {
			if !v.Field(i).CanSet() {
				continue
			}
			fieldChanged, err := resolveValue(v.Field(i), lookup, secretResolver)
			if err != nil {
				return false, err
			}
			changed = changed || fieldChanged
		}
		return changed, nil
	case reflect.Slice:
		changed := false
		for i := 0; i < v.Len(); i++ {
			itemChanged, err := resolveValue(v.Index(i), lookup, secretResolver)
			if err != nil {
				return false, err
			}
			changed = changed || itemChanged
		}
		return changed, nil
	case reflect.Map:
		changed := false
		iter := v.MapRange()
		for iter.Next() {
			k := iter.Key()
			val := iter.Value()
			switch val.Kind() {
			case reflect.String:
				resolved, itemChanged, err := resolveString(val.String(), lookup, secretResolver)
				if err != nil {
					return false, err
				}
				if itemChanged {
					v.SetMapIndex(k, reflect.ValueOf(resolved).Convert(v.Type().Elem()))
				}
				changed = changed || itemChanged
			case reflect.Interface:
				resolvedAny, itemChanged, err := resolveAny(val.Interface(), lookup, secretResolver)
				if err != nil {
					return false, err
				}
				if itemChanged {
					v.SetMapIndex(k, reflect.ValueOf(resolvedAny))
				}
				changed = changed || itemChanged
			}
		}
		return changed, nil
	default:
		return false, nil
	}
}

func resolveAny(value any, lookup map[string]any, secretResolver SecretResolver) (any, bool, error) {
	switch v := value.(type) {
	case string:
		return resolveString(v, lookup, secretResolver)
	case map[string]any:
		changed := false
		for k, item := range v {
			resolved, itemChanged, err := resolveAny(item, lookup, secretResolver)
			if err != nil {
				return nil, false, err
			}
			if itemChanged {
				v[k] = resolved
			}
			changed = changed || itemChanged
		}
		return v, changed, nil
	case []any:
		changed := false
		for i, item := range v {
			resolved, itemChanged, err := resolveAny(item, lookup, secretResolver)
			if err != nil {
				return nil, false, err
			}
			if itemChanged {
				v[i] = resolved
			}
			changed = changed || itemChanged
		}
		return v, changed, nil
	default:
		return value, false, nil
	}
}

func resolveString(input string, lookup map[string]any, secretResolver SecretResolver) (string, bool, error) {
	if !strings.Contains(input, "${") {
		return input, false, nil
	}

	var out strings.Builder
	changed := false
	remaining := input
	for {
		start := strings.Index(remaining, "${")
		if start < 0 {
			out.WriteString(remaining)
			break
		}
		out.WriteString(remaining[:start])
		exprStart := start + 2
		end := strings.Index(remaining[exprStart:], "}")
		if end < 0 {
			return "", false, fmt.Errorf("unterminated placeholder in %q", input)
		}
		expr := strings.TrimSpace(remaining[exprStart : exprStart+end])
		val, err := evalExpr(expr, lookup, secretResolver)
		if err != nil {
			return "", false, err
		}
		out.WriteString(val)
		changed = true
		remaining = remaining[exprStart+end+1:]
	}

	return out.String(), changed, nil
}

func evalExpr(expr string, lookup map[string]any, secretResolver SecretResolver) (string, error) {
	if _, c, t, f, ok := splitTernary(expr); ok {
		condVal, err := evalAtom(c, lookup, secretResolver)
		if err != nil {
			return "", err
		}
		if truthy(condVal) {
			return evalExpr(strings.TrimSpace(t), lookup, secretResolver)
		}
		return evalExpr(strings.TrimSpace(f), lookup, secretResolver)
	}
	v, err := evalAtom(expr, lookup, secretResolver)
	if err != nil {
		return "", err
	}
	return fmt.Sprint(v), nil
}

func evalAtom(expr string, lookup map[string]any, secretResolver SecretResolver) (any, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "", nil
	}
	if (strings.HasPrefix(expr, "\"") && strings.HasSuffix(expr, "\"")) || (strings.HasPrefix(expr, "'") && strings.HasSuffix(expr, "'")) {
		return expr[1 : len(expr)-1], nil
	}
	if expr == "true" {
		return true, nil
	}
	if expr == "false" {
		return false, nil
	}
	if n, err := strconv.Atoi(expr); err == nil {
		return n, nil
	}
	if provider, path, ok := parseSecretReference(expr); ok {
		if secretResolver == nil {
			return nil, fmt.Errorf("secret provider %q not configured", provider)
		}
		secretValue, err := secretResolver.Resolve(provider, path)
		if err != nil {
			return nil, err
		}
		return secretValue, nil
	}
	return lookupPath(expr, lookup)
}

func parseSecretReference(expr string) (provider string, path string, ok bool) {
	idx := strings.Index(expr, ":")
	if idx <= 0 || idx == len(expr)-1 {
		return "", "", false
	}

	provider = strings.TrimSpace(expr[:idx])
	path = strings.TrimSpace(expr[idx+1:])
	if provider == "" || path == "" {
		return "", "", false
	}

	for _, ch := range provider {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			continue
		}
		return "", "", false
	}

	return provider, path, true
}

func lookupPath(path string, lookup map[string]any) (any, error) {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return nil, fmt.Errorf("invalid path %q", path)
	}
	cur, ok := lookup[parts[0]]
	if !ok {
		return nil, fmt.Errorf("unknown application %q", parts[0])
	}
	for _, p := range parts[1:] {
		switch node := cur.(type) {
		case map[string]any:
			next, ok := node[p]
			if !ok {
				return nil, fmt.Errorf("unknown field %q in path %q", p, path)
			}
			cur = next
		case []any:
			idx, err := strconv.Atoi(p)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil, fmt.Errorf("invalid index %q in path %q", p, path)
			}
			cur = node[idx]
		default:
			return nil, fmt.Errorf("cannot descend into %q in path %q", p, path)
		}
	}
	return cur, nil
}

func splitTernary(expr string) (int, string, string, string, bool) {
	q := -1
	depth := 0
	quote := byte(0)
	for i := 0; i < len(expr); i++ {
		ch := expr[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}
		if ch == '(' {
			depth++
			continue
		}
		if ch == ')' && depth > 0 {
			depth--
			continue
		}
		if depth == 0 && ch == '?' {
			q = i
			break
		}
	}
	if q < 0 {
		return -1, "", "", "", false
	}

	depth = 0
	quote = 0
	colon := -1
	for i := q + 1; i < len(expr); i++ {
		ch := expr[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}
		if ch == '(' {
			depth++
			continue
		}
		if ch == ')' && depth > 0 {
			depth--
			continue
		}
		if depth == 0 && ch == ':' {
			colon = i
			break
		}
	}
	if colon < 0 {
		return -1, "", "", "", false
	}

	cond := strings.TrimSpace(expr[:q])
	ifTrue := strings.TrimSpace(expr[q+1 : colon])
	ifFalse := strings.TrimSpace(expr[colon+1:])
	return q, cond, ifTrue, ifFalse, true
}

func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.TrimSpace(strings.ToLower(t))
		return s != "" && s != "false" && s != "0" && s != "null"
	case int:
		return t != 0
	case int64:
		return t != 0
	case float64:
		return t != 0
	default:
		return t != nil
	}
}

func hasPlaceholders(app Application) bool {
	return valueHasPlaceholders(reflect.ValueOf(app))
}

func valueHasPlaceholders(v reflect.Value) bool {
	if !v.IsValid() {
		return false
	}
	switch v.Kind() {
	case reflect.String:
		return strings.Contains(v.String(), "${")
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if valueHasPlaceholders(v.Field(i)) {
				return true
			}
		}
		return false
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if valueHasPlaceholders(v.Index(i)) {
				return true
			}
		}
		return false
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			if valueHasPlaceholders(iter.Value()) {
				return true
			}
		}
		return false
	case reflect.Interface, reflect.Ptr:
		if v.IsNil() {
			return false
		}
		return valueHasPlaceholders(v.Elem())
	default:
		return false
	}
}
