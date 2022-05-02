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

package message

import (
	"context"

	fb "github.com/google/flatbuffers/go"

	"github.com/dolthub/dolt/go/gen/fb/serial"
	"github.com/dolthub/dolt/go/store/hash"
	"github.com/dolthub/dolt/go/store/pool"
	"github.com/dolthub/dolt/go/store/val"
)

const (
	// These constants are mirrored from serial.ProllyTreeNode.KeyOffsetsLength()
	// and serial.ProllyTreeNode.ValueOffsetsLength() respectively.
	// They are only as stable as the flatbuffers schemas that define them.
	keyOffsetsVOffset   = 6
	valueOffsetsVOffset = 12
)

func SerializeProllyMap(pool pool.BuffPool, keys, values [][]byte, level int, subtrees []uint64) Message {
	var (
		keyTups, keyOffs fb.UOffsetT
		valTups, valOffs fb.UOffsetT
		refArr, cardArr  fb.UOffsetT
	)

	keySz, valSz, bufSz := estimateBufferSize(keys, values, subtrees)
	b := getFlatbufferBuilder(pool, bufSz)

	// serialize keys and offsets
	keyTups = writeItemBytes(b, keys, keySz)
	serial.ProllyTreeNodeStartKeyOffsetsVector(b, len(keys)-1)
	keyOffs = writeItemOffsets(b, keys, keySz)

	if level == 0 {
		// serialize value tuples for leaf nodes
		valTups = writeItemBytes(b, values, valSz)
		serial.ProllyTreeNodeStartValueOffsetsVector(b, len(values)-1)
		valOffs = writeItemOffsets(b, values, valSz)
	} else {
		// serialize child refs and subtree counts for internal nodes
		refArr = writeItemBytes(b, values, valSz)
		cardArr = writeCountArray(b, subtrees)
	}

	// populate the node's vtable
	serial.ProllyTreeNodeStart(b)
	serial.ProllyTreeNodeAddKeyItems(b, keyTups)
	serial.ProllyTreeNodeAddKeyOffsets(b, keyOffs)
	if level == 0 {
		serial.ProllyTreeNodeAddValueItems(b, valTups)
		serial.ProllyTreeNodeAddValueOffsets(b, valOffs)
		serial.ProllyTreeNodeAddTreeCount(b, uint64(len(keys)))
	} else {
		serial.ProllyTreeNodeAddAddressArray(b, refArr)
		serial.ProllyTreeNodeAddSubtreeCounts(b, cardArr)
		serial.ProllyTreeNodeAddTreeCount(b, sumSubtrees(subtrees))
	}
	serial.ProllyTreeNodeAddKeyType(b, serial.ItemTypeTupleFormatAlpha)
	serial.ProllyTreeNodeAddValueType(b, serial.ItemTypeTupleFormatAlpha)
	serial.ProllyTreeNodeAddTreeLevel(b, uint8(level))
	b.FinishWithFileIdentifier(serial.ProllyTreeNodeEnd(b), []byte(serial.ProllyTreeNodeFileID))

	return b.FinishedBytes()
}

func sumSubtrees(subtrees []uint64) (sum uint64) {
	for i := range subtrees {
		sum += subtrees[i]
	}
	return
}

func getProllyMapKeys(msg Message) (keys val.SlicedBuffer) {
	pm := serial.GetRootAsProllyTreeNode(msg, 0)
	keys.Buf = pm.KeyItemsBytes()
	keys.Offs = getKeyOffsetsVector(pm)
	return
}

func getProllyMapValues(msg Message) (values val.SlicedBuffer) {
	pm := serial.GetRootAsProllyTreeNode(msg, 0)
	items := pm.ValueItemsBytes()
	if items != nil {
		values.Buf = items
		values.Offs = getValueOffsetsVector(pm)
	} else {
		values.Buf = pm.AddressArrayBytes()
		values.Offs = offsetsForAddressArray(values.Buf)
	}
	return
}

func walkProllyMapAddresses(ctx context.Context, msg Message, cb func(ctx context.Context, addr hash.Hash) error) error {
	pm := serial.GetRootAsProllyTreeNode(msg, 0)
	arr := pm.AddressArrayBytes()
	for i := 0; i < len(arr)/hash.ByteLen; i++ {
		addr := hash.New(arr[i*addrSz : (i+1)*addrSz])
		if err := cb(ctx, addr); err != nil {
			return err
		}
	}

	cnt := pm.ValueAddressOffsetsLength()
	arr2 := pm.ValueItemsBytes()
	for i := 0; i < cnt; i++ {
		o := pm.ValueAddressOffsets(i)
		addr := hash.New(arr[o : o+addrSz])
		if err := cb(ctx, addr); err != nil {
			return err
		}
	}
	assertFalse((arr != nil) && (arr2 != nil))
	return nil
}

func getProllyMapCount(msg Message) uint16 {
	pm := serial.GetRootAsProllyTreeNode(msg, 0)
	if pm.KeyItemsLength() == 0 {
		return 0
	}
	// zeroth offset ommitted from array
	return uint16(pm.KeyOffsetsLength() + 1)
}

func getProllyMapTreeLevel(msg Message) int {
	pm := serial.GetRootAsProllyTreeNode(msg, 0)
	return int(pm.TreeLevel())
}

func getProllyMapTreeCount(msg Message) int {
	pm := serial.GetRootAsProllyTreeNode(msg, 0)
	return int(pm.TreeCount())
}

func getProllyMapSubtrees(msg Message) []uint64 {
	cnt := getProllyMapCount(msg)
	pm := serial.GetRootAsProllyTreeNode(msg, 0)
	return readSubtreeCounts(int(cnt), pm.SubtreeCountsBytes())
}

func getKeyOffsetsVector(pm *serial.ProllyTreeNode) []byte {
	sz := pm.KeyOffsetsLength() * 2
	tab := pm.Table()
	vec := tab.Offset(keyOffsetsVOffset)
	start := int(tab.Vector(fb.UOffsetT(vec)))
	stop := start + sz

	return tab.Bytes[start:stop]
}

func getValueOffsetsVector(pm *serial.ProllyTreeNode) []byte {
	sz := pm.ValueOffsetsLength() * 2
	tab := pm.Table()
	vec := tab.Offset(valueOffsetsVOffset)
	start := int(tab.Vector(fb.UOffsetT(vec)))
	stop := start + sz

	return tab.Bytes[start:stop]
}
