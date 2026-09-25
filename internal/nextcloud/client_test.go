package nextcloud

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// fakeFileInfo is a minimal os.FileInfo for directory entries.
type fakeFileInfo struct {
	name  string
	isDir bool
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return 0 }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.isDir }
func (f fakeFileInfo) Sys() any           { return nil }

type fakeDav struct {
	files    map[string][]byte
	dirs     map[string][]os.FileInfo
	readDirN int // number of ReadDir calls, to assert memoisation
}

func (d *fakeDav) Read(p string) ([]byte, error) {
	if b, ok := d.files[p]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("404 %s", p)
}

func (d *fakeDav) ReadDir(p string) ([]os.FileInfo, error) {
	d.readDirN++
	if entries, ok := d.dirs[p]; ok {
		return entries, nil
	}
	return nil, fmt.Errorf("404 %s", p)
}

func TestReadRecipe_CaseInsensitiveFallback(t *testing.T) {
	recipeJSON := []byte(`{"name":"Chuka Pork Steaks"}`)
	dav := &fakeDav{
		files: map[string][]byte{
			// Only the correctly-cased path exists on the server.
			"/Recipes/Chuka Pork Steaks/recipe.json": recipeJSON,
		},
		dirs: map[string][]os.FileInfo{
			"/Recipes": {
				fakeFileInfo{name: "Chuka Pork Steaks", isDir: true},
				fakeFileInfo{name: "Gyudon", isDir: true},
			},
		},
	}
	c := &Client{dav: dav, recipesPath: "/Recipes"}

	// Plan lists it in the wrong case; lookup should still resolve it.
	r, err := c.ReadRecipe("Chuka pork steaks")
	if err != nil {
		t.Fatalf("ReadRecipe: %v", err)
	}
	if r.Name != "Chuka Pork Steaks" {
		t.Errorf("name = %q, want %q", r.Name, "Chuka Pork Steaks")
	}

	// Exact-case still works and must not need the directory listing.
	if _, err := c.ReadRecipe("Chuka Pork Steaks"); err != nil {
		t.Errorf("exact-case ReadRecipe: %v", err)
	}

	// A genuinely missing recipe still errors.
	if _, err := c.ReadRecipe("Nonexistent"); err == nil {
		t.Error("expected error for missing recipe")
	}

	if dav.readDirN != 1 {
		t.Errorf("ReadDir called %d times, want 1 (memoised)", dav.readDirN)
	}
}
