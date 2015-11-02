package les

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/access"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/trie"
)

// Tests that block headers can be retrieved from a remote chain based on user queries.
func TestGetBlockHeadersLes1(t *testing.T) { testGetBlockHeaders(t, 1) }

func testGetBlockHeaders(t *testing.T, protocol int) {
	pm, _ := newTestProtocolManagerMust(t, false, downloader.MaxHashFetch+15, nil)
	peer, _ := newTestPeer("peer", protocol, pm, true)
	defer peer.close()

	// Create a "random" unknown hash for testing
	var unknown common.Hash
	for i, _ := range unknown {
		unknown[i] = byte(i)
	}
	// Create a batch of tests for various scenarios
	limit := uint64(downloader.MaxHeaderFetch)
	tests := []struct {
		query  *getBlockHeadersData // The query to execute for header retrieval
		expect []common.Hash        // The hashes of the block whose headers are expected
	}{
		// A single random block should be retrievable by hash and number too
		{
			&getBlockHeadersData{Origin: hashOrNumber{Hash: pm.blockchain.GetBlockByNumber(limit/2, access.NoOdr).Hash()}, Amount: 1},
			[]common.Hash{pm.blockchain.GetBlockByNumber(limit/2, access.NoOdr).Hash()},
		}, {
			&getBlockHeadersData{Origin: hashOrNumber{Number: limit / 2}, Amount: 1},
			[]common.Hash{pm.blockchain.GetBlockByNumber(limit/2, access.NoOdr).Hash()},
		},
		// Multiple headers should be retrievable in both directions
		{
			&getBlockHeadersData{Origin: hashOrNumber{Number: limit / 2}, Amount: 3},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(limit/2, access.NoOdr).Hash(),
				pm.blockchain.GetBlockByNumber(limit/2+1, access.NoOdr).Hash(),
				pm.blockchain.GetBlockByNumber(limit/2+2, access.NoOdr).Hash(),
			},
		}, {
			&getBlockHeadersData{Origin: hashOrNumber{Number: limit / 2}, Amount: 3, Reverse: true},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(limit/2, access.NoOdr).Hash(),
				pm.blockchain.GetBlockByNumber(limit/2-1, access.NoOdr).Hash(),
				pm.blockchain.GetBlockByNumber(limit/2-2, access.NoOdr).Hash(),
			},
		},
		// Multiple headers with skip lists should be retrievable
		{
			&getBlockHeadersData{Origin: hashOrNumber{Number: limit / 2}, Skip: 3, Amount: 3},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(limit/2, access.NoOdr).Hash(),
				pm.blockchain.GetBlockByNumber(limit/2+4, access.NoOdr).Hash(),
				pm.blockchain.GetBlockByNumber(limit/2+8, access.NoOdr).Hash(),
			},
		}, {
			&getBlockHeadersData{Origin: hashOrNumber{Number: limit / 2}, Skip: 3, Amount: 3, Reverse: true},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(limit/2, access.NoOdr).Hash(),
				pm.blockchain.GetBlockByNumber(limit/2-4, access.NoOdr).Hash(),
				pm.blockchain.GetBlockByNumber(limit/2-8, access.NoOdr).Hash(),
			},
		},
		// The chain endpoints should be retrievable
		{
			&getBlockHeadersData{Origin: hashOrNumber{Number: 0}, Amount: 1},
			[]common.Hash{pm.blockchain.GetBlockByNumber(0, access.NoOdr).Hash()},
		}, {
			&getBlockHeadersData{Origin: hashOrNumber{Number: pm.blockchain.CurrentBlock().NumberU64()}, Amount: 1},
			[]common.Hash{pm.blockchain.CurrentBlock().Hash()},
		},
		// Ensure protocol limits are honored
		{
			&getBlockHeadersData{Origin: hashOrNumber{Number: pm.blockchain.CurrentBlock().NumberU64() - 1}, Amount: limit + 10, Reverse: true},
			pm.blockchain.GetBlockHashesFromHash(pm.blockchain.CurrentBlock().Hash(), limit),
		},
		// Check that requesting more than available is handled gracefully
		{
			&getBlockHeadersData{Origin: hashOrNumber{Number: pm.blockchain.CurrentBlock().NumberU64() - 4}, Skip: 3, Amount: 3},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(pm.blockchain.CurrentBlock().NumberU64()-4, access.NoOdr).Hash(),
				pm.blockchain.GetBlockByNumber(pm.blockchain.CurrentBlock().NumberU64(), access.NoOdr).Hash(),
			},
		}, {
			&getBlockHeadersData{Origin: hashOrNumber{Number: 4}, Skip: 3, Amount: 3, Reverse: true},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(4, access.NoOdr).Hash(),
				pm.blockchain.GetBlockByNumber(0, access.NoOdr).Hash(),
			},
		},
		// Check that requesting more than available is handled gracefully, even if mid skip
		{
			&getBlockHeadersData{Origin: hashOrNumber{Number: pm.blockchain.CurrentBlock().NumberU64() - 4}, Skip: 2, Amount: 3},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(pm.blockchain.CurrentBlock().NumberU64()-4, access.NoOdr).Hash(),
				pm.blockchain.GetBlockByNumber(pm.blockchain.CurrentBlock().NumberU64()-1, access.NoOdr).Hash(),
			},
		}, {
			&getBlockHeadersData{Origin: hashOrNumber{Number: 4}, Skip: 2, Amount: 3, Reverse: true},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(4, access.NoOdr).Hash(),
				pm.blockchain.GetBlockByNumber(1, access.NoOdr).Hash(),
			},
		},
		// Check that non existing headers aren't returned
		{
			&getBlockHeadersData{Origin: hashOrNumber{Hash: unknown}, Amount: 1},
			[]common.Hash{},
		}, {
			&getBlockHeadersData{Origin: hashOrNumber{Number: pm.blockchain.CurrentBlock().NumberU64() + 1}, Amount: 1},
			[]common.Hash{},
		},
	}
	// Run each of the tests and verify the results against the chain
	for i, tt := range tests {
		// Collect the headers to expect in the response
		headers := []*types.Header{}
		for _, hash := range tt.expect {
			headers = append(headers, pm.blockchain.GetBlock(hash, access.NoOdr).Header())
		}
		// Send the hash request and verify the response
		p2p.Send(peer.app, GetBlockHeadersMsg, tt.query)
		if err := p2p.ExpectMsg(peer.app, BlockHeadersMsg, headers); err != nil {
			t.Errorf("test %d: headers mismatch: %v", i, err)
		}
	}
}

// Tests that block contents can be retrieved from a remote chain based on their hashes.
func TestGetBlockBodiesLes1(t *testing.T) { testGetBlockBodies(t, 1) }

func testGetBlockBodies(t *testing.T, protocol int) {
	pm, _ := newTestProtocolManagerMust(t, false, downloader.MaxBlockFetch+15, nil)
	peer, _ := newTestPeer("peer", protocol, pm, true)
	defer peer.close()

	// Create a batch of tests for various scenarios
	limit := downloader.MaxBlockFetch
	tests := []struct {
		random    int           // Number of blocks to fetch randomly from the chain
		explicit  []common.Hash // Explicitly requested blocks
		available []bool        // Availability of explicitly requested blocks
		expected  int           // Total number of existing blocks to expect
	}{
		{1, nil, nil, 1},                                                         // A single random block should be retrievable
		{10, nil, nil, 10},                                                       // Multiple random blocks should be retrievable
		{limit, nil, nil, limit},                                                 // The maximum possible blocks should be retrievable
		{limit + 1, nil, nil, limit},                                             // No more than the possible block count should be returned
		{0, []common.Hash{pm.blockchain.Genesis().Hash()}, []bool{true}, 1},      // The genesis block should be retrievable
		{0, []common.Hash{pm.blockchain.CurrentBlock().Hash()}, []bool{true}, 1}, // The chains head block should be retrievable
		{0, []common.Hash{common.Hash{}}, []bool{false}, 0},                      // A non existent block should not be returned

		// Existing and non-existing blocks interleaved should not cause problems
		{0, []common.Hash{
			common.Hash{},
			pm.blockchain.GetBlockByNumber(1, access.NoOdr).Hash(),
			common.Hash{},
			pm.blockchain.GetBlockByNumber(10, access.NoOdr).Hash(),
			common.Hash{},
			pm.blockchain.GetBlockByNumber(100, access.NoOdr).Hash(),
			common.Hash{},
		}, []bool{false, true, false, true, false, true, false}, 3},
	}
	// Run each of the tests and verify the results against the chain
	for i, tt := range tests {
		// Collect the hashes to request, and the response to expect
		hashes, seen := []common.Hash{}, make(map[int64]bool)
		bodies := []*types.Body{}

		for j := 0; j < tt.random; j++ {
			for {
				num := rand.Int63n(int64(pm.blockchain.CurrentBlock().NumberU64()))
				if !seen[num] {
					seen[num] = true

					block := pm.blockchain.GetBlockByNumber(uint64(num), access.NoOdr)
					hashes = append(hashes, block.Hash())
					if len(bodies) < tt.expected {
						bodies = append(bodies, &types.Body{Transactions: block.Transactions(), Uncles: block.Uncles()})
					}
					break
				}
			}
		}
		for j, hash := range tt.explicit {
			hashes = append(hashes, hash)
			if tt.available[j] && len(bodies) < tt.expected {
				block := pm.blockchain.GetBlock(hash, access.NoOdr)
				bodies = append(bodies, &types.Body{Transactions: block.Transactions(), Uncles: block.Uncles()})
			}
		}
		// Send the hash request and verify the response
		p2p.Send(peer.app, GetBlockBodiesMsg, hashes)
		if err := p2p.ExpectMsg(peer.app, BlockBodiesMsg, bodies); err != nil {
			t.Errorf("test %d: bodies mismatch: %v", i, err)
		}
	}
}

// Tests that the node state database can be retrieved based on hashes.
func TestGetNodeDataLes1(t *testing.T) { testGetNodeData(t, 1) }

func testGetNodeData(t *testing.T, protocol int) {
	// Assemble the test environment
	pm, _ := newTestProtocolManagerMust(t, false, 4, testChainGen)
	peer, _ := newTestPeer("peer", protocol, pm, true)
	defer peer.close()

	// Fetch for now the entire chain db
	hashes := []common.Hash{}
	for _, key := range pm.chainAccess.Db().(*ethdb.MemDatabase).Keys() {
		if len(key) == len(common.Hash{}) {
			hashes = append(hashes, common.BytesToHash(key))
		}
	}
	p2p.Send(peer.app, GetNodeDataMsg, hashes)
	msg, err := peer.app.ReadMsg()
	if err != nil {
		t.Fatalf("failed to read node data response: %v", err)
	}
	if msg.Code != NodeDataMsg {
		t.Fatalf("response packet code mismatch: have %x, want %x", msg.Code, 0x0c)
	}
	var data [][]byte
	if err := msg.Decode(&data); err != nil {
		t.Fatalf("failed to decode response node data: %v", err)
	}
	// Verify that all hashes correspond to the requested data, and reconstruct a state tree
	for i, want := range hashes {
		if hash := crypto.Sha3Hash(data[i]); hash != want {
			fmt.Errorf("data hash mismatch: have %x, want %x", hash, want)
		}
	}
	statedb, _ := ethdb.NewMemDatabase()
	for i := 0; i < len(data); i++ {
		statedb.Put(hashes[i].Bytes(), data[i])
	}
	accounts := []common.Address{testBankAddress, acc1Addr, acc2Addr}
	for i := uint64(0); i <= pm.blockchain.CurrentBlock().NumberU64(); i++ {
		trie, _ := state.New(pm.blockchain.GetBlockByNumber(i, access.NoOdr).Root(), access.NewDbChainAccess(statedb), access.NullCtx)

		for j, acc := range accounts {
			state, _ := pm.blockchain.State(access.NullCtx)
			bw := state.GetBalance(acc)
			bh := trie.GetBalance(acc)

			if (bw != nil && bh == nil) || (bw == nil && bh != nil) {
				t.Errorf("test %d, account %d: balance mismatch: have %v, want %v", i, j, bh, bw)
			}
			if bw != nil && bh != nil && bw.Cmp(bw) != 0 {
				t.Errorf("test %d, account %d: balance mismatch: have %v, want %v", i, j, bh, bw)
			}
		}
	}
}

// Tests that the transaction receipts can be retrieved based on hashes.
func TestGetReceiptLes1(t *testing.T) { testGetReceipt(t, 1) }

func testGetReceipt(t *testing.T, protocol int) {
	// Assemble the test environment
	pm, _ := newTestProtocolManagerMust(t, false, 4, testChainGen)
	peer, _ := newTestPeer("peer", protocol, pm, true)
	defer peer.close()

	// Collect the hashes to request, and the response to expect
	hashes, receipts := []common.Hash{}, []types.Receipts{}
	for i := uint64(0); i <= pm.blockchain.CurrentBlock().NumberU64(); i++ {
		block := pm.blockchain.GetBlockByNumber(i, access.NoOdr)

		hashes = append(hashes, block.Hash())
		receipts = append(receipts, core.GetBlockReceipts(pm.chainAccess, block.Hash(), access.NoOdr))
	}
	// Send the hash request and verify the response
	p2p.Send(peer.app, GetReceiptsMsg, hashes)
	if err := p2p.ExpectMsg(peer.app, ReceiptsMsg, receipts); err != nil {
		t.Errorf("receipts mismatch: %v", err)
	}
}

// Tests that trie merkle proofs can be retrieved
func TestGetProofsLes1(t *testing.T) { testGetReceipt(t, 1) }

func testGetProofs(t *testing.T, protocol int) {
	// Assemble the test environment
	pm, ca := newTestProtocolManagerMust(t, false, 4, testChainGen)
	db := ca.Db()
	peer, _ := newTestPeer("peer", protocol, pm, true)
	defer peer.close()

	var proofreqs []access.ProofReq
	var proofs []trie.MerkleProof

	accounts := []common.Address{testBankAddress, acc1Addr, acc2Addr, common.Address{}}
	for i := uint64(0); i <= pm.blockchain.CurrentBlock().NumberU64(); i++ {
		root := pm.blockchain.GetBlockByNumber(i, access.NoOdr).Root()
		trie, _ := trie.NewSecure(root, db, nil)

		for _, acc := range accounts {
			req := access.ProofReq{
				Root: root,
				Key:  acc[:],
			}
			proofreqs = append(proofreqs, req)

			proof := trie.Prove(acc[:])
			proofs = append(proofs, proof)
		}
	}
	// Send the proof request and verify the response
	p2p.Send(peer.app, GetProofsMsg, proofreqs)
	if err := p2p.ExpectMsg(peer.app, ProofsMsg, proofs); err != nil {
		t.Errorf("receipts mismatch: %v", err)
	}
}