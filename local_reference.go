package actionlint

import (
	"fmt"
	"path/filepath"
	"strings"
)

// selfRepositoryUsesLocalSpec converts a reference to the running workflow's
// repository into the local metadata cache's repository-relative form.
func selfRepositoryUsesLocalSpec(spec string) (string, error) {
	if !strings.HasPrefix(spec, "$/") || len(spec) == 2 {
		return "", fmt.Errorf("path is missing")
	}
	path := spec[2:]
	if strings.ContainsRune(path, '@') {
		return "", fmt.Errorf("self-repository references cannot specify a ref")
	}
	if strings.ContainsRune(path, '\\') || !filepath.IsLocal(filepath.FromSlash(path)) {
		return "", fmt.Errorf("path must stay within the repository")
	}
	return "./" + filepath.ToSlash(filepath.Clean(filepath.FromSlash(path))), nil
}
