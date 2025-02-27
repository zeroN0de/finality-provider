//go:build e2e
// +build e2e

package e2etest

import (
	"math/rand"
	"testing"
	"time"

	"github.com/babylonchain/babylon/testutil/datagen"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/stretchr/testify/require"

	"github.com/babylonchain/finality-provider/finality-provider/service"
	"github.com/babylonchain/finality-provider/types"
)

var (
	stakingTime   = uint16(100)
	stakingAmount = int64(20000)
)

// TestFinalityProviderLifeCycle tests the whole life cycle of a finality-provider
// creation -> registration -> randomness commitment ->
// activation with BTC delegation and Covenant sig ->
// vote submission -> block finalization
func TestFinalityProviderLifeCycle(t *testing.T) {
	tm, fpInsList := StartManagerWithFinalityProvider(t, 1)
	defer tm.Stop(t)

	fpIns := fpInsList[0]

	// check the public randomness is committed
	tm.WaitForFpPubRandCommitted(t, fpIns)

	// send a BTC delegation
	_ = tm.InsertBTCDelegation(t, []*btcec.PublicKey{fpIns.MustGetBtcPk()}, stakingTime, stakingAmount)

	// check the BTC delegation is pending
	dels := tm.WaitForNPendingDels(t, 1)

	// send covenant sigs
	tm.InsertCovenantSigForDelegation(t, dels[0])

	// check the BTC delegation is active
	_ = tm.WaitForNActiveDels(t, 1)

	// check the last voted block is finalized
	lastVotedHeight := tm.WaitForFpVoteCast(t, fpIns)
	tm.CheckBlockFinalization(t, lastVotedHeight, 1)
	t.Logf("the block at height %v is finalized", lastVotedHeight)
}

// TestDoubleSigning tests the attack scenario where the finality-provider
// sends a finality vote over a conflicting block
// in this case, the BTC private key should be extracted by Babylon
func TestDoubleSigning(t *testing.T) {
	tm, fpInsList := StartManagerWithFinalityProvider(t, 1)
	defer tm.Stop(t)

	fpIns := fpInsList[0]

	// check the public randomness is committed
	tm.WaitForFpPubRandCommitted(t, fpIns)

	// send a BTC delegation
	_ = tm.InsertBTCDelegation(t, []*btcec.PublicKey{fpIns.MustGetBtcPk()}, stakingTime, stakingAmount)

	// check the BTC delegation is pending
	dels := tm.WaitForNPendingDels(t, 1)

	// send covenant sigs
	tm.InsertCovenantSigForDelegation(t, dels[0])

	// check the BTC delegation is active
	_ = tm.WaitForNActiveDels(t, 1)

	// check the last voted block is finalized
	lastVotedHeight := tm.WaitForFpVoteCast(t, fpIns)
	tm.CheckBlockFinalization(t, lastVotedHeight, 1)
	t.Logf("the block at height %v is finalized", lastVotedHeight)

	finalizedBlocks := tm.WaitForNFinalizedBlocks(t, 1)

	// attack: manually submit a finality vote over a conflicting block
	// to trigger the extraction of finality-provider's private key
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := &types.BlockInfo{
		Height: finalizedBlocks[0].Height,
		Hash:   datagen.GenRandomByteArray(r, 32),
	}
	_, extractedKey, err := fpIns.TestSubmitFinalitySignatureAndExtractPrivKey(b)
	require.NoError(t, err)
	require.NotNil(t, extractedKey)
	localKey := tm.GetFpPrivKey(t, fpIns.GetBtcPkBIP340().MustMarshal())
	require.True(t, localKey.Key.Equals(&extractedKey.Key) || localKey.Key.Negate().Equals(&extractedKey.Key))

	t.Logf("the equivocation attack is successful")

	tm.WaitForFpShutDown(t, fpIns.GetBtcPkBIP340())

	// try to start all the finality providers and the slashed one should not be restarted
	err = tm.Fpa.StartHandlingAll()
	require.NoError(t, err)
	fps := tm.Fpa.ListFinalityProviderInstances()
	require.Equal(t, 0, len(fps))
}

// TestMultipleFinalityProviders tests starting with multiple finality providers
func TestMultipleFinalityProviders(t *testing.T) {
	n := 3
	tm, fpInstances := StartManagerWithFinalityProvider(t, n)
	defer tm.Stop(t)

	// submit BTC delegations for each finality-provider
	for _, fpIns := range fpInstances {
		tm.Wg.Add(1)
		go func(fpi *service.FinalityProviderInstance) {
			defer tm.Wg.Done()
			// check the public randomness is committed
			tm.WaitForFpPubRandCommitted(t, fpi)
			// send a BTC delegation
			_ = tm.InsertBTCDelegation(t, []*btcec.PublicKey{fpi.MustGetBtcPk()}, stakingTime, stakingAmount)
		}(fpIns)
	}
	tm.Wg.Wait()

	// check the BTC delegations are pending
	dels := tm.WaitForNPendingDels(t, n)
	require.Equal(t, n, len(dels))

	// send covenant sigs to each of the delegations
	for _, d := range dels {
		// send covenant sigs
		tm.InsertCovenantSigForDelegation(t, d)
	}

	// check the BTC delegations are active
	_ = tm.WaitForNActiveDels(t, n)

	// check there's a block finalized
	_ = tm.WaitForNFinalizedBlocks(t, 1)
}

// TestFastSync tests the fast sync process where the finality-provider is terminated and restarted with fast sync
func TestFastSync(t *testing.T) {
	tm, fpInsList := StartManagerWithFinalityProvider(t, 1)
	defer tm.Stop(t)

	fpIns := fpInsList[0]

	// check the public randomness is committed
	tm.WaitForFpPubRandCommitted(t, fpIns)

	// send a BTC delegation
	_ = tm.InsertBTCDelegation(t, []*btcec.PublicKey{fpIns.MustGetBtcPk()}, stakingTime, stakingAmount)

	// check the BTC delegation is pending
	dels := tm.WaitForNPendingDels(t, 1)

	// send covenant sigs
	tm.InsertCovenantSigForDelegation(t, dels[0])

	// check the BTC delegation is active
	_ = tm.WaitForNActiveDels(t, 1)

	// check the last voted block is finalized
	lastVotedHeight := tm.WaitForFpVoteCast(t, fpIns)
	tm.CheckBlockFinalization(t, lastVotedHeight, 1)

	t.Logf("the block at height %v is finalized", lastVotedHeight)

	var finalizedBlocks []*types.BlockInfo
	finalizedBlocks = tm.WaitForNFinalizedBlocks(t, 1)

	n := 3
	// stop the finality-provider for a few blocks then restart to trigger the fast sync
	tm.FpConfig.FastSyncGap = uint64(n)
	tm.StopAndRestartFpAfterNBlocks(t, n, fpIns)

	// check there are n+1 blocks finalized
	finalizedBlocks = tm.WaitForNFinalizedBlocks(t, n+1)
	finalizedHeight := finalizedBlocks[0].Height
	t.Logf("the latest finalized block is at %v", finalizedHeight)

	// check if the fast sync works by checking if the gap is not more than 1
	currentHeaderRes, err := tm.BBNClient.QueryBestBlock()
	currentHeight := currentHeaderRes.Height
	t.Logf("the current block is at %v", currentHeight)
	require.NoError(t, err)
	require.True(t, currentHeight < finalizedHeight+uint64(n))
}

// TestFastSync_DuplicateVotes covers a special case when the finality signature
// has inconsistent view of last voted height with the Babylon node and during
// fast-sync it submits a batch of finality sigs, one of which is rejected due
// to duplicate error
// this test covers this case by starting 3 finality providers, 2 of which
// are stopped after gaining voting power to simulate the case where no blocks
// are finalized. Then we let one of the finality providers "forget" the last
// voted height and restart all the finality providers, expecting it them to
// catch up and finalize new blocks
func TestFastSync_DuplicateVotes(t *testing.T) {
	tm, fpInsList := StartManagerWithFinalityProvider(t, 3)
	defer tm.Stop(t)

	fpIns1 := fpInsList[0]
	fpIns2 := fpInsList[1]
	fpIns3 := fpInsList[2]

	// check the public randomness is committed
	tm.WaitForFpPubRandCommitted(t, fpIns1)
	tm.WaitForFpPubRandCommitted(t, fpIns2)
	tm.WaitForFpPubRandCommitted(t, fpIns3)

	// send 3 BTC delegations to empower the three finality providers
	_ = tm.InsertBTCDelegation(t, []*btcec.PublicKey{fpIns1.MustGetBtcPk()}, stakingTime, stakingAmount)
	_ = tm.InsertBTCDelegation(t, []*btcec.PublicKey{fpIns2.MustGetBtcPk()}, stakingTime, stakingAmount)
	_ = tm.InsertBTCDelegation(t, []*btcec.PublicKey{fpIns3.MustGetBtcPk()}, stakingTime, stakingAmount)

	// check the BTC delegations are pending
	dels := tm.WaitForNPendingDels(t, 3)

	// send covenant sigs to each delegation
	tm.InsertCovenantSigForDelegation(t, dels[0])
	tm.InsertCovenantSigForDelegation(t, dels[1])
	tm.InsertCovenantSigForDelegation(t, dels[2])

	// check the BTC delegations are active
	_ = tm.WaitForNActiveDels(t, 3)

	// stop 2 of the finality providers so that no blocks will be finalized
	err := fpIns2.Stop()
	require.NoError(t, err)
	err = fpIns3.Stop()
	require.NoError(t, err)

	// make sure fp1 has cast a finality vote
	// and then make it "forget" the last voted height
	lastVotedHeight := tm.WaitForFpVoteCast(t, fpIns1)
	fpIns1.MustUpdateStateAfterFinalitySigSubmission(lastVotedHeight - 1)

	// stop fp1, restarts all the fps after 3 blocks for them to catch up
	// and expect a block will be finalized
	n := 3
	tm.FpConfig.FastSyncGap = uint64(n)
	tm.StopAndRestartFpAfterNBlocks(t, n, fpIns1)
	err = fpIns2.Start()
	require.NoError(t, err)
	err = fpIns3.Start()
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		finalizedBlocks := tm.WaitForNFinalizedBlocks(t, 1)
		return finalizedBlocks[0].Height > lastVotedHeight
	}, eventuallyWaitTimeOut, eventuallyPollTime)
}
