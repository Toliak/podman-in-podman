package pinp

import (
	"reflect"
	"testing"
)

func TestInfoAdditionalImageStores(t *testing.T) {
	info := &Info{}
	info.Store.GraphOptions = map[string]any{
		"overlay.additionalImageStores": []any{"/a/store", "/var/lib/shared"},
		"overlay.imagestore":            "/legacy/store",
		"overlay.mount_program": map[string]any{
			"Executable": "/usr/local/bin/fuse-overlayfs",
		},
	}
	got := info.AdditionalImageStores()
	want := []string{"/a/store", "/var/lib/shared", "/legacy/store"}
	if !reflect.DeepEqual(got, want) && !reflect.DeepEqual(reverse(got), want) {
		t.Fatalf("AdditionalImageStores = %v, want %v", got, want)
	}
	var nilInfo *Info
	if nilInfo.AdditionalImageStores() != nil {
		t.Fatal("nil info should return nil")
	}
}

func reverse(in []string) []string {
	out := make([]string, len(in))
	for i := range in {
		out[len(in)-1-i] = in[i]
	}
	return out
}
