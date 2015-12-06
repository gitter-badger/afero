package afero

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizePath(t *testing.T) {
	normalizePath(testSubDir)
}

func TestMemMapFsRelAbs(t *testing.T) {
	filename1 := "dotRelTestFile"
	filename2 := "dotAbsTestFile"
	err := os.Mkdir(testDir, 0755)
	if err != nil {
		t.Error(err)
	}
	fs := new(MemMapFs)
	err = os.Chdir(testDir)
	if err != nil {
		t.Error("Can't Chdir(", testDir, "):", err)
	}
	//fmt.Println("Working directory:")
	//fmt.Println(os.Getwd())

	// create file with relative path
	f, err := fs.Create(filepath.Join(".", filename1))
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("DotTestFile content")
	f.Close()

	// try to open with absolute path
	_, err = fs.Open(filepath.Join(testDir, filename1))
	if err != nil {
		fmt.Println("--- fs1 ---")
		fs.List()
		fmt.Println("-----------")
		t.Error("fs.Open absolute path error:", err)
	}

	// create file with absolute path
	f, err = fs.Create(filepath.Join(testDir, filename2))
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("DotTestFile content")
	f.Close()

	// try to open with relative path
	_, err = fs.Open(filepath.Join(".", filename2))
	if err != nil {
		fmt.Println("--- fs2 ---", filepath.Join(".", filename2))
		fs.List()
		fmt.Println("-----------")
		t.Error("fs.Open relative path error:", err)
	}
	err = os.RemoveAll(testDir)
	if err != nil {
		t.Error(err)
	}
}

func xTestNormalizePath(t *testing.T) {
	type test struct {
		input    string
		expected string
	}

	data := []test{
		{".", "/"},
		{".", "/"},
		{"./", "/"},
		{"..", "/"},
		{"../", "/"},
		{"./..", "/"},
		{"./../", "/"},
	}

	for i, d := range data {
		cpath := normalizePath(d.input)
		if d.expected != cpath {
			t.Errorf("Test %d failed. Expected %s got %s", i, d.expected, cpath)
		}
	}
}
