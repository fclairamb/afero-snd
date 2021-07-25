package snd

import (
	"os"
	"testing"
)

func TestFileOps(t *testing.T) {
	fs, assert := GetFsAuto(t)

	file, err := fs.OpenFile("test2", os.O_CREATE|os.O_WRONLY, 0755)
	assert.NoError(err)

	defer func() {
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := file.WriteString("test2"); err != nil {
		t.Fatal(err)
	}

	if err := file.Sync(); err != nil {
		t.Fatal(err)
	}

	if err := file.Truncate(2); err != nil {
		t.Fatal(err)
	}

	if info, err := fs.Stat("test2"); err != nil {
		t.Fatal(err)
	} else if info.Size() != 2 {
		t.Fatalf("File size should be 2 and was %v", info.Size())
	}
}
