package testing

import (
	"context"
	"strconv"
	"testing"

	"github.com/aloknerurkar/bee-fs/pkg/store"
	"github.com/ethersphere/bee/pkg/storage"
	testingc "github.com/ethersphere/bee/pkg/storage/testing"
)

func RunTests(t *testing.T, st store.PutGetter) {
	t.Helper()

	ctx := context.Background()

	ch := testingc.GenerateTestRandomChunk()
	_, err := st.Put(ctx, storage.ModePutUpload, ch)
	if err != nil {
		t.Fatal(err)
	}
	chResult, err := st.Get(ctx, storage.ModeGetRequest, ch.Address())
	if err != nil {
		t.Fatal(err)
	}
	if !ch.Equal(chResult) {
		t.Fatal("chunk mismatch")
	}
}

func RunBackupTests(t *testing.T, st store.BackupStore) {
	t.Helper()

	ctx := context.Background()

	tag, err := st.CreateTag(ctx)
	if err != nil {
		t.Fatal(err)
	}

	tagInfo, err := st.GetTag(ctx, tag)
	if err != nil {
		t.Fatal(err)
	}

	if strconv.Itoa(int(tagInfo.Uid)) != tag {
		t.Fatalf("invalid tag returned on initial get exp %s found %s", tag, strconv.Itoa(int(tagInfo.Uid)))
	}

	if tagInfo.Total != 0 || tagInfo.Synced != 0 || tagInfo.Processed != 0 {
		t.Fatalf("incorrect tag details %+v", tagInfo)
	}

	ch := testingc.GenerateTestRandomChunk()
	err = st.PutWithTag(ctx, storage.ModePutUpload, tag, ch)
	if err != nil {
		t.Fatal(err)
	}
	chResult, err := st.Get(ctx, storage.ModeGetRequest, ch.Address())
	if err != nil {
		t.Fatal(err)
	}
	if !ch.Equal(chResult) {
		t.Fatal("chunk mismatch")
	}

	tagInfo, err = st.GetTag(ctx, tag)
	if err != nil {
		t.Fatal(err)
	}

	if strconv.Itoa(int(tagInfo.Uid)) != tag {
		t.Fatalf("invalid tag returned exp %s found %s", tag, strconv.Itoa(int(tagInfo.Uid)))
	}

	if tagInfo.Total != 0 || tagInfo.Synced != 0 || tagInfo.Processed != 1 {
		t.Fatalf("incorrect tag details %+v", tagInfo)
	}
}
