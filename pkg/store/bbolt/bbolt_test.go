package boltstore_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/aloknerurkar/bee-fs/pkg/store/bbolt"
	storetesting "github.com/aloknerurkar/bee-fs/pkg/store/testing"
)

func TestBoltStore(t *testing.T) {
	mntDir, err := ioutil.TempDir("", "tmpbolt")
	st, err := boltstore.NewBoltStore(mntDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(mntDir)

	storetesting.RunTests(t, st)
}
