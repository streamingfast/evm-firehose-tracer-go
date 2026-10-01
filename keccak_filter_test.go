package firehose

import (
	"encoding/hex"
	"testing"

	"math/big"

	"github.com/holiman/uint256"
	eth "github.com/streamingfast/eth-go"
	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	"github.com/stretchr/testify/assert"
)

// recordKeccak hashes preimage, records it on call and returns the hash.
func recordKeccak(call *pbeth.Call, preimage []byte) [32]byte {
	hash := [32]byte(eth.Keccak256(preimage))
	if call.KeccakPreimages == nil {
		call.KeccakPreimages = map[string]string{}
	}
	call.KeccakPreimages[hex.EncodeToString(hash[:])] = hex.EncodeToString(preimage)
	return hash
}

func storeKey(call *pbeth.Call, key [32]byte) {
	newValue := [32]byte{31: 1}
	call.StorageChanges = append(call.StorageChanges, &pbeth.StorageChange{
		Address:  []byte{19: 0xaa},
		Key:      key[:],
		OldValue: make([]byte, 32),
		NewValue: newValue[:],
	})
}

func word(v uint64) []byte {
	return new(big.Int).SetUint64(v).FillBytes(make([]byte, 32))
}

func concat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func addToHash(h [32]byte, offset *uint256.Int) [32]byte {
	return new(uint256.Int).Add(new(uint256.Int).SetBytes32(h[:]), offset).Bytes32()
}

// retainStorageSlotPreimages moves the preimages the tests put in the maps into a recorded
// list, as the tracer holds them during execution, then attaches the kept ones back.
func retainStorageSlotPreimages(calls []*pbeth.Call) {
	var recorded []recordedPreimage
	for position, call := range calls {
		call.Index = uint32(position + 1)
		for hash, preimage := range call.KeccakPreimages {
			h, _ := hex.DecodeString(hash)
			data, _ := hex.DecodeString(preimage)
			recorded = append(recorded, recordedPreimage{call.Index, [32]byte(h), data})
		}
		call.KeccakPreimages = nil
	}
	attachStorageSlotPreimages(calls, recorded)
}

func keptKeccaks(calls []*pbeth.Call) []string {
	var out []string
	for _, call := range calls {
		for hash := range call.KeccakPreimages {
			out = append(out, hash)
		}
	}
	return out
}

func hexHashes(hashes ...[32]byte) []string {
	out := make([]string, len(hashes))
	for i, h := range hashes {
		out[i] = hex.EncodeToString(h[:])
	}
	return out
}

func TestRetainStorageSlotPreimages_KeepsMappingSlotAndDropsUnrelated(t *testing.T) {
	call := &pbeth.Call{}
	slot := recordKeccak(call, concat(word(1), word(0)))
	recordKeccak(call, concat(word(9), word(9)))
	storeKey(call, slot)

	calls := []*pbeth.Call{call}
	retainStorageSlotPreimages(calls)

	assert.ElementsMatch(t, hexHashes(slot), keptKeccaks(calls))
}

func TestRetainStorageSlotPreimages_KeepsArrayElementAndStructFieldSlots(t *testing.T) {
	call := &pbeth.Call{}
	base := recordKeccak(call, word(3))
	storeKey(call, addToHash(base, uint256.NewInt(7)))

	calls := []*pbeth.Call{call}
	retainStorageSlotPreimages(calls)

	assert.ElementsMatch(t, hexHashes(base), keptKeccaks(calls))
}

func TestRetainStorageSlotPreimages_DropsHashTooFarBelowStorageKey(t *testing.T) {
	call := &pbeth.Call{}
	base := recordKeccak(call, word(3))
	storeKey(call, addToHash(base, new(uint256.Int).Lsh(uint256.NewInt(1), 64)))

	calls := []*pbeth.Call{call}
	retainStorageSlotPreimages(calls)

	assert.Empty(t, keptKeccaks(calls))
	assert.Nil(t, call.KeccakPreimages)
}

func TestRetainStorageSlotPreimages_FollowsNestedMappingsAndStringKeys(t *testing.T) {
	call := &pbeth.Call{}
	// mapping(uint => mapping(uint => T)) at slot 2: keccak(k2 . keccak(k1 . 2))
	inner := recordKeccak(call, concat(word(1), word(2)))
	outer := recordKeccak(call, concat(word(5), inner[:]))
	// mapping(string => T) at slot 4 nested under a mapping: keccak("abc" . keccak(k . 4))
	stringParent := recordKeccak(call, concat(word(8), word(4)))
	stringSlot := recordKeccak(call, concat([]byte("abc"), stringParent[:]))
	storeKey(call, outer)
	storeKey(call, stringSlot)

	calls := []*pbeth.Call{call}
	retainStorageSlotPreimages(calls)

	assert.ElementsMatch(t, hexHashes(inner, outer, stringParent, stringSlot), keptKeccaks(calls))
}

func TestRetainStorageSlotPreimages_FollowsMappingInsideStructInMapping(t *testing.T) {
	// struct Pool { uint total; mapping(address => uint) shares; }
	// mapping(uint => Pool) pools at slot 3: pools[id].shares[user] is at
	// keccak(user . (keccak(id . 3) + 1)).
	call := &pbeth.Call{}
	pool := recordKeccak(call, concat(word(7), word(3)))
	sharesSlot := addToHash(pool, uint256.NewInt(1))
	share := recordKeccak(call, concat(word(0xee), sharesSlot[:]))
	storeKey(call, share)

	calls := []*pbeth.Call{call}
	retainStorageSlotPreimages(calls)

	assert.ElementsMatch(t, hexHashes(pool, share), keptKeccaks(calls))
}

func TestRetainStorageSlotPreimages_KeepsPreimageRecordedInAnotherCall(t *testing.T) {
	hashing, writing := &pbeth.Call{}, &pbeth.Call{}
	slot := recordKeccak(hashing, concat(word(1), word(0)))
	storeKey(writing, slot)

	calls := []*pbeth.Call{hashing, writing}
	retainStorageSlotPreimages(calls)

	assert.ElementsMatch(t, hexHashes(slot), keptKeccaks(calls))
}

func TestRetainStorageSlotPreimages_StopsAfterMaxDepth(t *testing.T) {
	call := &pbeth.Call{}
	chain := [][32]byte{recordKeccak(call, concat(word(1), word(0)))}
	for i := 0; i < keccakFilterMaxDepth+1; i++ {
		parent := chain[len(chain)-1]
		chain = append(chain, recordKeccak(call, concat(word(uint64(i+10)), parent[:])))
	}
	storeKey(call, chain[len(chain)-1])

	calls := []*pbeth.Call{call}
	retainStorageSlotPreimages(calls)

	// The storage key's own entry is depth 0, then keccakFilterMaxDepth levels below it.
	assert.ElementsMatch(t, hexHashes(chain[1:]...), keptKeccaks(calls))
}
