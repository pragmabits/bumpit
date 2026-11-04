package nodejs

import (
	"reflect"
	"testing"
)

func TestPackagesCarryTheirVersion(t *testing.T) {
	t.Parallel()

	top, tracked := writeTree(t, map[string]string{
		"web/widget/package.json":  `{"name": "widget", "version": "2.0.0"}`,
		"web/private/package.json": `{"name": "private", "version": "1.0.0", "private": true}`,
		"web/bare/package.json":    `{"name": "bare"}`,
		"web/string/package.json":  `{"name": "string", "version": "1.0.0", "private": "true"}`,
		"web/zero/package.json":    `{"name": "zero", "version": "1.0.0", "private": 0}`,
	})

	var got []Package
	for _, found := range Read(top, tracked).Packages() {
		got = append(got, Package{Directory: found.Directory, Version: found.Version, Private: found.Private})
	}
	want := []Package{
		{Directory: "web/bare"},
		{Directory: "web/private", Version: "1.0.0", Private: true},
		{Directory: "web/string", Version: "1.0.0", Private: true},
		{Directory: "web/widget", Version: "2.0.0"},
		{Directory: "web/zero", Version: "1.0.0"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Packages = %+v\nwant %+v", got, want)
	}
}
