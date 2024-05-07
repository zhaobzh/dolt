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

package merge

import (
	"fmt"

	"github.com/dolthub/go-mysql-server/sql"

	"github.com/dolthub/dolt/go/libraries/doltcore/doltdb"
	"github.com/dolthub/dolt/go/libraries/doltcore/table/editor"
)

// Revert is a convenience function for a three-way merge. In particular, given some root and a collection of commits
// that are all parents of the root value, this applies a three-way merge with the following characteristics (assuming
// a commit is HEAD~1):
//
// Base:   HEAD~1
// Ours:   root
// Theirs: HEAD~2
//
// The root is updated with the merged result, and this process is repeated for each commit given, in the order given.
// Currently, we error on conflicts or constraint violations generated by the merge.
func Revert(ctx *sql.Context, ddb *doltdb.DoltDB, root doltdb.RootValue, commits []*doltdb.Commit, opts editor.Options) (doltdb.RootValue, string, error) {
	revertMessage := "Revert"

	for _, cm := range commits {
		if len(cm.DatasParents()) == 0 {
			h, err := cm.HashOf()
			if err != nil {
				return nil, "", err
			}
			return nil, "", fmt.Errorf("cannot revert commit with no parents (%s)", h.String())
		}
	}

	for i, baseCommit := range commits {
		if i > 0 {
			revertMessage += " and"
		}
		baseRoot, err := baseCommit.GetRootValue(ctx)
		if err != nil {
			return nil, "", err
		}
		baseMeta, err := baseCommit.GetCommitMeta(ctx)
		if err != nil {
			return nil, "", err
		}
		revertMessage = fmt.Sprintf(`%s "%s"`, revertMessage, baseMeta.Description)

		optCmt, err := ddb.ResolveParent(ctx, baseCommit, 0)
		if err != nil {
			return nil, "", err
		}
		parentCM, ok := optCmt.ToCommit()
		if !ok {
			return nil, "", doltdb.ErrGhostCommitEncountered
		}

		theirRoot, err := parentCM.GetRootValue(ctx)
		if err != nil {
			return nil, "", err
		}

		var result *Result
		result, err = MergeRoots(ctx, root, theirRoot, baseRoot, parentCM, baseCommit, opts, MergeOpts{IsCherryPick: false})
		if err != nil {
			return nil, "", err
		}
		root = result.Root

		if ok, err := result.Root.HasConflicts(ctx); err != nil {
			return nil, "", err
		} else if ok {
			return nil, "", fmt.Errorf("revert currently does not handle conflicts")
		}
		if ok, err := result.Root.HasConstraintViolations(ctx); err != nil {
			return nil, "", err
		} else if ok {
			return nil, "", fmt.Errorf("revert currently does not handle constraint violations")
		}
	}

	return root, revertMessage, nil
}
