// Copyright 2021 Dolthub, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package env

import (
	"context"
	"fmt"
	"time"

	"github.com/dolthub/dolt/go/store/chunks"
	"github.com/dolthub/dolt/go/store/hash"

	"github.com/dolthub/dolt/go/libraries/doltcore/doltdb"
	"github.com/dolthub/dolt/go/libraries/doltcore/doltdocs"
	"github.com/dolthub/dolt/go/libraries/doltcore/ref"
)

func NewMemoryDbData(ctx context.Context) (DbData, error) {
	ddb, err := NewMemoryDoltDB(ctx)
	if err != nil {
		return DbData{}, err
	}

	rs, err := NewMemoryRepoState(ctx, ddb)
	if err != nil {
		return DbData{}, err
	}

	return DbData{
		Ddb: ddb,
		Rsw: rs,
		Rsr: rs,
		Drw: rs,
	}, nil
}

func NewMemoryDoltDB(ctx context.Context) (*doltdb.DoltDB, error) {
	ts := &chunks.TestStorage{}
	cs := ts.NewViewWithDefaultFormat()
	ddb := doltdb.DoltDBFromCS(cs)

	err := ddb.WriteEmptyRepo(ctx, "memory", "memory")
	if err != nil {
		return nil, err
	}

	return ddb, nil
}

func NewMemoryRepoState(ctx context.Context, ddb *doltdb.DoltDB) (MemoryRepoState, error) {
	bb, err := ddb.GetBranches(ctx)
	if err != nil {
		return MemoryRepoState{}, err
	}

	return MemoryRepoState{
		DoltDB: ddb,
		Head:   bb[0],
	}, nil
}

type MemoryRepoState struct {
	DoltDB *doltdb.DoltDB
	Head   ref.DoltRef
}

var _ RepoStateReader = MemoryRepoState{}
var _ RepoStateWriter = MemoryRepoState{}
var _ DocsReadWriter = MemoryRepoState{}

func (m MemoryRepoState) CWBHeadRef() ref.DoltRef {
	return m.Head
}

func (m MemoryRepoState) CWBHeadSpec() *doltdb.CommitSpec {
	spec, err := doltdb.NewCommitSpec(m.CWBHeadRef().GetPath())
	if err != nil {
		panic(err)
	}
	return spec
}

func (m MemoryRepoState) UpdateStagedRoot(ctx context.Context, newRoot *doltdb.RootValue) error {
	var h hash.Hash
	var wsRef ref.WorkingSetRef

	ws, err := m.WorkingSet(ctx)
	if err == doltdb.ErrWorkingSetNotFound {
		// first time updating root
		wsRef, err = ref.WorkingSetRefForHead(m.CWBHeadRef())
		if err != nil {
			return err
		}
		ws = doltdb.EmptyWorkingSet(wsRef).WithWorkingRoot(newRoot).WithStagedRoot(newRoot)
	} else if err != nil {
		return err
	} else {
		h, err = ws.HashOf()
		if err != nil {
			return err
		}

		wsRef = ws.Ref()
	}

	return m.DoltDB.UpdateWorkingSet(ctx, wsRef, ws.WithStagedRoot(newRoot), h, m.workingSetMeta())
}

func (m MemoryRepoState) UpdateWorkingRoot(ctx context.Context, newRoot *doltdb.RootValue) error {
	var h hash.Hash
	var wsRef ref.WorkingSetRef

	ws, err := m.WorkingSet(ctx)
	if err == doltdb.ErrWorkingSetNotFound {
		// first time updating root
		wsRef, err = ref.WorkingSetRefForHead(m.CWBHeadRef())
		if err != nil {
			return err
		}
		ws = doltdb.EmptyWorkingSet(wsRef).WithWorkingRoot(newRoot).WithStagedRoot(newRoot)
	} else if err != nil {
		return err
	} else {
		h, err = ws.HashOf()
		if err != nil {
			return err
		}

		wsRef = ws.Ref()
	}

	return m.DoltDB.UpdateWorkingSet(ctx, wsRef, ws.WithWorkingRoot(newRoot), h, m.workingSetMeta())
}

func (m MemoryRepoState) WorkingSet(ctx context.Context) (*doltdb.WorkingSet, error) {
	workingSetRef, err := ref.WorkingSetRefForHead(m.CWBHeadRef())
	if err != nil {
		return nil, err
	}

	workingSet, err := m.DoltDB.ResolveWorkingSet(ctx, workingSetRef)
	if err != nil {
		return nil, err
	}

	return workingSet, nil
}

func (m MemoryRepoState) workingSetMeta() *doltdb.WorkingSetMeta {
	return &doltdb.WorkingSetMeta{
		Timestamp:   uint64(time.Now().Unix()),
		Description: "updated from dolt environment",
	}
}

func (m MemoryRepoState) SetCWBHeadRef(_ context.Context, r ref.MarshalableRef) (err error) {
	m.Head = r.Ref
	return
}

func (m MemoryRepoState) GetRemotes() (map[string]Remote, error) {
	return make(map[string]Remote), nil
}

func (m MemoryRepoState) AddRemote(name string, url string, fetchSpecs []string, params map[string]string) error {
	return fmt.Errorf("cannot insert a remote in a memory database")
}

func (m MemoryRepoState) RemoveRemote(ctx context.Context, name string) error {
	return fmt.Errorf("cannot delete a remote from a memory database")
}

func (m MemoryRepoState) GetDocsOnDisk(docNames ...string) (doltdocs.Docs, error) {
	return nil, fmt.Errorf("cannot get docs from a memory database")
}

func (m MemoryRepoState) WriteDocsToDisk(docs doltdocs.Docs) error {
	return fmt.Errorf("cannot write docs to a memory database")
}
