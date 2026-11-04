package helm

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRead(t *testing.T) {
	t.Parallel()

	top := t.TempDir()
	files := map[string]string{
		"deploy/chart/Chart.yaml":             "apiVersion: v2\nname: chart\nversion: 1.4.0\n",
		"deploy/chart/charts/sub/Chart.yaml":  "apiVersion: v2\nname: sub\nversion: 0.1.0\n",
		"Chart.yaml":                          "apiVersion: v2\nname: top\nversion: 2.0.0\n",
		"loose/charts/alone/Chart.yaml":       "apiVersion: v2\nname: alone\nversion: 0.3.0\n",
		"unversioned/Chart.yaml":              "apiVersion: v2\nname: unversioned\n",
		"deploy/chart/templates/service.yaml": "kind: Service\n",
	}
	var tracked []string
	for name, content := range files {
		writeFile(t, top, name, content)
		tracked = append(tracked, name)
	}
	tracked = append(tracked, "deleted/Chart.yaml")

	charts, unreadable := Read(top, tracked)
	if len(unreadable) > 0 {
		t.Fatalf("Read set aside %+v", unreadable)
	}
	want := []Chart{
		{Directory: ".", Version: "2.0.0"},
		{Directory: "deploy/chart", Version: "1.4.0"},
		{Directory: "loose/charts/alone", Version: "0.3.0"},
	}
	if !reflect.DeepEqual(charts, want) {
		t.Errorf("Read = %+v\nwant %+v", charts, want)
	}
}

func TestReadSetsAsideAnUnreadableChart(t *testing.T) {
	t.Parallel()

	top := t.TempDir()
	writeFile(t, top, "chart/Chart.yaml", "version: [\n")
	writeFile(t, top, "good/Chart.yaml", "version: 1.0.0\n")
	charts, unreadable := Read(top, []string{"chart/Chart.yaml", "good/Chart.yaml"})
	if !reflect.DeepEqual(charts, []Chart{{Directory: "good", Version: "1.0.0"}}) {
		t.Errorf("Read = %+v, want the readable chart", charts)
	}
	if len(unreadable) != 1 || unreadable[0].Directory != "chart" || unreadable[0].Err == nil {
		t.Errorf("unreadable = %+v, want chart", unreadable)
	}
}

func writeFile(t *testing.T, top, name, content string) {
	t.Helper()

	file := filepath.Join(top, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
}
