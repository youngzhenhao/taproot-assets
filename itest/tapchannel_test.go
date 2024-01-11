package itest

import (
	"bytes"
	"context"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/davecgh/go-spew/spew"
	"github.com/lightninglabs/lndclient"
	tap "github.com/lightninglabs/taproot-assets"
	"github.com/lightninglabs/taproot-assets/address"
	"github.com/lightninglabs/taproot-assets/asset"
	"github.com/lightninglabs/taproot-assets/commitment"
	"github.com/lightninglabs/taproot-assets/fn"
	"github.com/lightninglabs/taproot-assets/internal/test"
	"github.com/lightninglabs/taproot-assets/proof"
	"github.com/lightninglabs/taproot-assets/tapchannel"
	"github.com/lightninglabs/taproot-assets/tapfreighter"
	"github.com/lightninglabs/taproot-assets/tappsbt"
	"github.com/lightninglabs/taproot-assets/taprpc"
	wrpc "github.com/lightninglabs/taproot-assets/taprpc/assetwalletrpc"
	"github.com/lightninglabs/taproot-assets/taprpc/mintrpc"
	"github.com/lightninglabs/taproot-assets/taprpc/universerpc"
	"github.com/lightninglabs/taproot-assets/tapscript"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/keychain"
	"github.com/lightningnetwork/lnd/lnrpc/signrpc"
	"github.com/lightningnetwork/lnd/lnrpc/walletrpc"
	"github.com/lightningnetwork/lnd/lntest/node"
	"github.com/lightningnetwork/lnd/lnwallet/chainfee"
	"github.com/lightningnetwork/lnd/lnwallet/chanfunding"
	"github.com/lightningnetwork/lnd/shachain"
	"github.com/stretchr/testify/require"
)

// Execution plan for the integration mini-poc:
//
// Cover the following phases:
// - channel funding
// - channel signing
// - commitment signing
// - HTLC signing
// - revocation
// - channel closing
//
//
// General scenario info:
//
// - initiator is B
// - dual-funded (on asset level only, to increase coverage)
// - BTC:
//   - 102k sats from B
//   - 1k sats from A
// - Assets:
//   - 400 LUSD A (400 asset ID X [available 700])
//   - 500 LUSD B (400 asset ID Y [available 400], 100 asset ID Z [available 500])
//
// funding (BTC-level):
// - 1 BTC inputs with only sats
// - 3 BTC inputs with assets and sats
// - 1 BTC output with funding output (assets and sats)
// - 2 BTC outputs with change (assets and sats)
//
// funding (asset-level):
// - distribution according to funding algorithm below
// - 3 vPSBTs, one for X, one for Y, one for Z
//   - X: split, 400 to channel, 300 to change A
//   - Y: full value, 400 to channel
//   - Z: split, 100 to channel, 500 to change B
// - recipients are identified according to output index
//
//
// commitment 0 (BTC-level):
// - 1 BTC input with assets and sats
// - 2 BTC outputs with assets and sats
//
// commitment 0 (asset-level):
// - distribution according to commitment algorithm below
// - 3 vPSBTs, one for X, one for Y, one for Z (might differ depending on sorting of asset ID)
//   - X: full value, 400 to A
//   - Y: full value, 400 to B
//   - Z: full value, 100 to B
// - recipients are identified according to output index
//
//
// commitment 1 with HTLC (BTC-level):
// - 1 BTC input with assets and sats
// - 3 BTC outputs with assets and sats
//
// commitment 1 with HTLC (asset-level):
// - distribution according to algorithm implemented in tapchannel.DistributeCoins
// - 3 vPSBTs, one for X, one for Y, one for Z (might differ depending on sorting of asset ID)
//   - X: split, 350 to A, 50 to H
//   - Y: full value, 400 to B
//   - Z: full value, 100 to B
// - recipients are identified according to output index
//
//
// commitment 2 (BTC-level):
// - 1 BTC input with assets and sats
// - 2 BTC outputs with assets and sats
//
// commitment 2 (asset-level):
// - distribution according to algorithm implemented in tapchannel.DistributeCoins
// - 3 vPSBTs, one for X, one for Y, one for Z (might differ depending on sorting of asset ID)
//   - X: split, 350 to A, 50 to B
//   - Y: full value, 400 to B
//   - Z: full value, 100 to B
// - recipients are identified according to output index
//
//
// cooperative close (BTC-level):
// - 1 BTC input with assets and sats
// - 2 BTC outputs with assets and sats
//
// cooperative close (asset-level):
// - distribution according to commitment algorithm below
// - 3 vPSBTs, one for X, one for Y, one for Z (might differ depending on sorting of asset ID)
//   - X: split, 350 to A, 50 to B
//   - Y: full value, 400 to B
//   - Z: full value, 100 to B
// - recipients are identified according to output index

// TODOs (later on):
// - have passive assets in one of the funding inputs
// - dual fund of assets should also mean dual fund of BTC, otherwise responder
//   gets gifted 1k sats in the asset change output

const (
	lusdName     = "LUSD"
	lusdMetaData = "proudly minted by the Lightning Network"
)

var (
	lusdBatchTemplate = &mintrpc.MintAssetRequest{
		Asset: &mintrpc.MintAsset{
			AssetType: taprpc.AssetType_NORMAL,
			Name:      lusdName,
			AssetMeta: &taprpc.AssetMeta{
				Data: []byte(lusdMetaData),
			},
			Amount:       1000,
			AssetVersion: taprpc.AssetVersion_ASSET_VERSION_V1,
		},
	}
)

// txAssetProof corresponds to the tx_asset_proof p2p message sent from one
// lnd to another before initiating a channel funding request.
type txAssetProof struct {
	tempChannelID [32]byte
	assetID       []byte
	amount        uint64
	proof         []byte
}

// openChannel corresponds to the open_channel p2p message sent from one lnd to
// another to initiate a channel funding request.
type openChannel struct {
	tempChannelID           [32]byte
	fundingAmount           uint64
	fundingPubKey           []byte
	revocationBasePoint     []byte
	paymentBasePoint        []byte
	delayedPaymentBasePoint []byte
	htlcBasePoint           []byte
	firstCommitmentPoint    []byte
	nextLocalNonce          []byte
	tapNextLocalNonces      [][]byte
	tapAssetRoot            *universerpc.MerkleSumNode
}

// acceptChannel corresponds to the accept_channel p2p message sent from one lnd
// to another to accept a channel funding request.
type acceptChannel struct {
	tempChannelID           [32]byte
	fundingAmount           uint64
	fundingPubKey           []byte
	revocationBasePoint     []byte
	paymentBasePoint        []byte
	delayedPaymentBasePoint []byte
	htlcBasePoint           []byte
	firstCommitmentPoint    []byte
	nextLocalNonce          []byte
	tapNextLocalNonces      [][]byte
	tapAssetRoot            *universerpc.MerkleSumNode
}

// fundingCreated corresponds to the funding_created p2p message sent from one
// lnd to another to indicate that the channel funding transaction has been
// created.
type fundingCreated struct {
	tempChannelID                 [32]byte
	fundingTxid                   []byte
	fundingOutput                 uint32
	partialSignatureWithNonce     []byte
	tapPartialSignaturesWithNonce map[asset.ID][]byte
}

// fundingSigned corresponds to the funding_signed p2p message sent from one lnd
// to another to indicate that the channel funding transaction has been signed.
type fundingSigned struct {
	channelID                     [32]byte
	partialSignatureWithNonce     []byte
	tapPartialSignaturesWithNonce map[asset.ID][]byte
}

// channelReady corresponds to the channel_ready p2p message sent from one lnd
// to another to indicate that the channel is ready to be used.
type channelReady struct {
	channelID                [32]byte
	secondPerCommitmentPoint []byte
	nextLocalNonce           []byte
}

// testSimulateTaprootAssetsChannelFlow tests the flow of a taproot assets
// channel funding, commitment transaction creation, HTLC addition and
// settlement and channel cooperative closing on the transaction level.
func testSimulateTaprootAssetsChannelFlow(t *harnessTest) {
	// --------------------------------
	// Setup phase
	// --------------------------------

	ctxb := context.Background()
	ctxt, cancel := context.WithTimeout(ctxb, defaultWaitTimeout)
	defer cancel()

	// We are going to use LUSD throughout the test, which stands for
	// Lightning USD. We are going to create three batches of LUSD to make
	// sure we can cover all the intricacies of having fungible assets with
	// different asset IDs in a single channel. The three asset IDs created
	// by the three batches will be called X, Y and Z.
	firstBatchReq := CopyRequest(lusdBatchTemplate)
	firstBatchReq.Asset.NewGroupedAsset = true
	firstBatchReq.Asset.AssetMeta.Data = []byte(
		lusdMetaData + " batch X",
	)
	X := MintAssetsConfirmBatch(
		t.t, t.lndHarness.Miner.Client, t.tapd,
		[]*mintrpc.MintAssetRequest{firstBatchReq},
	)[0]

	secondBatchReq := CopyRequest(lusdBatchTemplate)
	secondBatchReq.Asset.GroupedAsset = true
	secondBatchReq.Asset.GroupKey = X.AssetGroup.TweakedGroupKey
	secondBatchReq.Asset.AssetMeta.Data = []byte(
		lusdMetaData + " batch Y",
	)
	Y := MintAssetsConfirmBatch(
		t.t, t.lndHarness.Miner.Client, t.tapd,
		[]*mintrpc.MintAssetRequest{secondBatchReq},
	)[0]

	thirdBatchReq := CopyRequest(secondBatchReq)
	thirdBatchReq.Asset.AssetMeta.Data = []byte(
		lusdMetaData + " batch Z",
	)
	Z := MintAssetsConfirmBatch(
		t.t, t.lndHarness.Miner.Client, t.tapd,
		[]*mintrpc.MintAssetRequest{thirdBatchReq},
	)[0]

	t.Logf("Minted three batches. First batch: %v\nSecond batch: %v\n"+
		"Third batch: %v\n", toJSON(t.t, X), toJSON(t.t, Y),
		toJSON(t.t, Z))

	// Now we create two totally separate tapd nodes, a and b that will
	// simulate the two parties of a channel. A is connected to lnd Alice
	// and b to lnd Bob. We'll send assets to both of them.
	var (
		lndA        = t.lndHarness.Alice
		lndB        = t.lndHarness.Bob
		chainParams = &address.RegressionNetTap
	)
	A := setupTapdHarness(t.t, t, lndA, t.universeServer)
	defer func() {
		require.NoError(t.t, A.stop(!*noDelete))
	}()
	B := setupTapdHarness(t.t, t, lndB, t.universeServer)
	defer func() {
		require.NoError(t.t, B.stop(!*noDelete))
	}()

	// We now distribute 700 units of the first tranche to A and 400 and
	// 500 units of the second and third tranche respectively to B.
	sendAssetsAndAssert(t, t.tapd, A, X, 700, 1, 1)
	sendAssetsAndAssert(t, t.tapd, B, Y, 400, 2, 1)
	sendAssetsAndAssert(t, t.tapd, B, Z, 500, 3, 2)

	startAssetsA, err := A.ListAssets(ctxt, &taprpc.ListAssetRequest{})
	require.NoError(t.t, err)
	startAssetsB, err := B.ListAssets(ctxt, &taprpc.ListAssetRequest{})
	require.NoError(t.t, err)
	fundingAssetX := findAsset(startAssetsA.Assets, X.AssetGenesis.AssetId)
	fundingAssetY := findAsset(startAssetsB.Assets, Y.AssetGenesis.AssetId)
	fundingAssetZ := findAsset(startAssetsB.Assets, Z.AssetGenesis.AssetId)
	t.Logf("Sent assets, A is contributing: %v\nB is contributing: "+
		"%v\n%v\n", toJSON(t.t, fundingAssetX),
		toJSON(t.t, fundingAssetY), toJSON(t.t, fundingAssetZ))

	var (
		assetIDX, assetIDY, assetIDZ asset.ID
	)
	copy(assetIDX[:], X.AssetGenesis.AssetId)
	copy(assetIDY[:], Y.AssetGenesis.AssetId)
	copy(assetIDZ[:], Z.AssetGenesis.AssetId)

	// --------------------------------
	// Channel funding phase
	// --------------------------------
	const (
		btcFundingAmountA = 1_000
		btcFundingAmountB = 102_000

		assetTotalAmountA = 700
		assetTotalAmountB = 900

		assetFundingAmountA = 400
		assetFundingAmountB = 400 + 100
	)

	// tx_asset_proof (initiator)
	//
	// We simulate two tx_asset_proof messages from B to A that precede the
	// channel funding request from B. B wants to contribute 500 LUSD from
	// their balance, which results in 400 units from tranche 2 and 100
	// units from tranche 3.
	var (
		tempChanID         = test.RandHash()
		proofFileY         = fetchProofFile(t.t, B, fundingAssetY)
		contributionProofY = extractLastProof(t.t, proofFileY)
		proofFileZ         = fetchProofFile(t.t, B, fundingAssetZ)
		contributionProofZ = extractLastProof(t.t, proofFileZ)
		assetContributionB = []*txAssetProof{
			{
				tempChannelID: tempChanID,
				assetID:       Y.AssetGenesis.AssetId,
				amount:        400,
				proof: serializeProof(
					t.t, contributionProofY,
				),
			},
			{
				tempChannelID: tempChanID,
				assetID:       Z.AssetGenesis.AssetId,
				amount:        100,
				proof: serializeProof(
					t.t, contributionProofZ,
				),
			},
		}
	)
	t.Logf("B is contributing: %#v\n", assetContributionB)

	// tx_asset_proof (responder)
	//
	// Even though dual funding isn't supported on the protocol level, we
	// want to simulate it in this test case to make sure we cover as many
	// edge cases as possible. We simulate one tx_asset_proof messages from
	// A to B that indicates A wants to contribute 400 LUSD from their
	// balance.
	var (
		proofFileX         = fetchProofFile(t.t, A, fundingAssetX)
		contributionProofX = extractLastProof(t.t, proofFileX)
		assetContributionA = []*txAssetProof{
			{
				tempChannelID: tempChanID,
				assetID:       X.AssetGenesis.AssetId,
				amount:        assetFundingAmountA,
				proof: serializeProof(
					t.t, contributionProofX,
				),
			},
		}
	)
	t.Logf("A is contributing: %#v\n", assetContributionA)

	// Calculate tapAssetRoot from the input assets.
	var (
		tapAssetRoot = assetRoot(t.t, []*taprpc.Asset{X, Y, Z})
	)

	// open_channel (initiator)
	var (
		bFundingKey = deriveKey(t.t, lndB, keychain.KeyFamilyMultiSig)
		bRevKey     = deriveKey(t.t, lndB, keychain.KeyFamilyRevocationBase)
		bPayKey     = deriveKey(t.t, lndB, keychain.KeyFamilyPaymentBase)
		bDelayKey   = deriveKey(t.t, lndB, keychain.KeyFamilyDelayBase)
		bHtlcKey    = deriveKey(t.t, lndB, keychain.KeyFamilyHtlcBase)

		bRevocationProducer = createRevocationProducer(
			t.t, lndB, bRevKey, bFundingKey.PubKey,
		)
		bCommitPoint     = commitPointAt(t.t, bRevocationProducer, 0)
		bFundingNonceOpt = musig2.WithPublicKey(bFundingKey.PubKey)
		bBtcNonces, _    = musig2.GenNonces(bFundingNonceOpt)
		bTapNoncesX, _   = musig2.GenNonces(bFundingNonceOpt)
		bTapNoncesY, _   = musig2.GenNonces(bFundingNonceOpt)
		bTapNoncesZ, _   = musig2.GenNonces(bFundingNonceOpt)

		bFundingInput   = getUtxo(t.t, lndB, btcFundingAmountB)
		btcInputAmountB = bFundingInput.Value
		feeRate         = chainfee.FeePerKwFloor
	)
	openChannelMsg := &openChannel{
		tempChannelID:           tempChanID,
		fundingAmount:           btcFundingAmountB,
		fundingPubKey:           pubKeyBytes(bFundingKey),
		revocationBasePoint:     pubKeyBytes(bRevKey),
		paymentBasePoint:        pubKeyBytes(bPayKey),
		delayedPaymentBasePoint: pubKeyBytes(bDelayKey),
		htlcBasePoint:           pubKeyBytes(bHtlcKey),
		firstCommitmentPoint:    bCommitPoint.SerializeCompressed(),
		nextLocalNonce:          bBtcNonces.PubNonce[:],
		tapNextLocalNonces: [][]byte{
			bTapNoncesX.PubNonce[:],
			bTapNoncesY.PubNonce[:],
			bTapNoncesZ.PubNonce[:],
		},
		tapAssetRoot: tapAssetRoot,
	}
	t.Logf("open_channel: %#v\n", openChannelMsg)

	// accept_channel (responder)
	var (
		aFundingKey = deriveKey(t.t, lndA, keychain.KeyFamilyMultiSig)
		aRevKey     = deriveKey(t.t, lndA, keychain.KeyFamilyRevocationBase)
		aPayKey     = deriveKey(t.t, lndA, keychain.KeyFamilyPaymentBase)
		aDelayKey   = deriveKey(t.t, lndA, keychain.KeyFamilyDelayBase)
		aHtlcKey    = deriveKey(t.t, lndA, keychain.KeyFamilyHtlcBase)

		aRevocationProducer = createRevocationProducer(
			t.t, lndA, aRevKey, aFundingKey.PubKey,
		)
		aCommitPoint     = commitPointAt(t.t, aRevocationProducer, 0)
		aFundingNonceOpt = musig2.WithPublicKey(aFundingKey.PubKey)
		aBtcNonces, _    = musig2.GenNonces(aFundingNonceOpt)
		aTapNoncesX, _   = musig2.GenNonces(aFundingNonceOpt)
		aTapNoncesY, _   = musig2.GenNonces(aFundingNonceOpt)
		aTapNoncesZ, _   = musig2.GenNonces(aFundingNonceOpt)
	)
	acceptChannelMsg := &acceptChannel{
		tempChannelID:           tempChanID,
		fundingAmount:           btcFundingAmountA,
		fundingPubKey:           pubKeyBytes(aFundingKey),
		revocationBasePoint:     pubKeyBytes(aRevKey),
		paymentBasePoint:        pubKeyBytes(aPayKey),
		delayedPaymentBasePoint: pubKeyBytes(aDelayKey),
		htlcBasePoint:           pubKeyBytes(aHtlcKey),
		firstCommitmentPoint:    aCommitPoint.SerializeCompressed(),
		nextLocalNonce:          aBtcNonces.PubNonce[:],
		tapNextLocalNonces: [][]byte{
			aTapNoncesX.PubNonce[:],
			aTapNoncesY.PubNonce[:],
			aTapNoncesZ.PubNonce[:],
		},
		tapAssetRoot: tapAssetRoot,
	}
	t.Logf("accept_channel: %#v\n", acceptChannelMsg)

	// funding_created (initiator)
	tapAggregatedKey, err := input.MuSig2CombineKeys(
		input.MuSig2Version100RC2, []*btcec.PublicKey{
			aFundingKey.PubKey, bFundingKey.PubKey,
		}, true, &input.MuSig2Tweaks{TaprootBIP0086Tweak: true},
	)
	require.NoError(t.t, err)

	var (
		aChangeScriptKey, aChangeIntKey = deriveKeys(t.t, A)
		bChangeScriptKey, bChangeIntKey = deriveKeys(t.t, A)
		btcAggregatedInternalKey        = tapAggregatedKey.PreTweakedKey
	)

	// Funding transaction: We have 3 asset outputs:
	//	0: channel funding output
	//	1: asset and BTC change for initiator (B)
	//	2: asset change for responder (A)
	fundingAllocation := []tapchannel.Allocation{
		{
			// Channel funding output.
			OutputIndex: 0,
			InternalKey: btcAggregatedInternalKey,
			ScriptKey: asset.NewScriptKey(
				tapAggregatedKey.FinalKey,
			),
			AssetVersion: asset.V1,
			Amount:       assetFundingAmountA + assetFundingAmountB,
			BtcAmount:    btcFundingAmountA + btcFundingAmountB,
		},
		{
			// Asset and BTC change for initiator (B).
			OutputIndex:  1,
			InternalKey:  bChangeIntKey.PubKey,
			ScriptKey:    bChangeScriptKey,
			AssetVersion: asset.V1,
			Amount:       assetTotalAmountB - assetFundingAmountB,
			BtcAmount: btcutil.Amount(
				btcInputAmountB - btcFundingAmountB,
			),
			BtcSubtractFees: true,
		},
		{
			// Asset change for responder (A).
			OutputIndex:  2,
			InternalKey:  aChangeIntKey.PubKey,
			ScriptKey:    aChangeScriptKey,
			AssetVersion: asset.V1,
			Amount:       assetTotalAmountA - assetFundingAmountA,
			BtcAmount:    tapscript.DummyAmtSats,
		},
	}

	contributionInputProofs := []*proof.Proof{
		contributionProofX, contributionProofY, contributionProofZ,
	}
	fundingVirtualPackets, err := tapchannel.DistributeCoins(
		ctxt, contributionInputProofs, fundingAllocation, assetIDX,
		assetIDY, assetIDZ, chainParams,
	)
	require.NoError(t.t, err)

	// We now have the finished virtual packets (but without signatures
	// yet), which is everything we need to create the funding transaction.
	// Since we're using asset version 1, we can commit the assets without
	// witnesses. So we can create the funding transaction now and then ask
	// the daemon to map the virtual packets to it.
	fundingPacket, err := tapfreighter.CreateFundedAnchorTX(
		fundingAllocation, fundingVirtualPackets, []*chanfunding.Coin{
			bFundingInput,
		}, feeRate,
	)
	require.NoError(t.t, err)

	// In order for us to be able to use SignPsbt later on to sign for the
	// asset carrying inputs, we now need to add the derivation information
	// of the internal keys to the funding packet.
	addAnchorDerivationInfo(t.t, A, fundingPacket.FundedPsbt.Pkt)
	addAnchorDerivationInfo(t.t, B, fundingPacket.FundedPsbt.Pkt)

	fundingPacketB64, err := fundingPacket.FundedPsbt.Pkt.B64Encode()
	require.NoError(t.t, err)

	t.Logf("BTC level funding packet: %v\n", fundingPacketB64)

	// We now sign the asset level virtual transactions that pay into the
	// funding transaction.
	fundingInputProofs := make(map[asset.ID]*proof.Proof)
	for idx := range fundingVirtualPackets {
		vPacket := fundingVirtualPackets[idx]
		vPacketPsbt, err := vPacket.B64Encode()
		require.NoError(t.t, err)

		assetID := vPacket.Inputs[0].PrevID.ID
		t.Logf("Asset level funding packet for asset ID %v: %v\n",
			assetID.String(), vPacketPsbt)

		signer := B
		if bytes.Equal(assetID[:], X.AssetGenesis.AssetId) {
			signer = A
		}

		vPacket = signVirtualPsbt(t.t, signer, vPacket)

		// The funding output is at index 0.
		fundingProof, err := tapfreighter.CreateProofSuffix(
			fundingPacket, vPacket, 0, fundingVirtualPackets,
		)
		require.NoError(t.t, err)

		fundingInputProofs[assetID] = fundingProof
		fundingVirtualPackets[idx] = vPacket
	}

	// Commitment transaction: We have 2 asset outputs:
	//	0: commitment transaction for initiator (B), 500 LUSD, 100k sats
	//	1: commitment transaction for responder (A), 400 LUSD, 1k sats
	// NOTE: We only simulate the initiator's commitment transaction here,
	// so to_local is for B and to_remote is for A.
	bLocalKey := input.TweakPubKey(bDelayKey.PubKey, aCommitPoint)
	bRevokeKey := input.DeriveRevocationPubkey(aRevKey.PubKey, aCommitPoint)
	bToLocalTree, err := input.NewLocalCommitScriptTree(
		2016, bLocalKey, bRevokeKey,
	)
	require.NoError(t.t, err)
	bToLocalTreeRoot := bToLocalTree.TapscriptTree.RootNode

	bToRemoteTree, err := input.NewRemoteCommitScriptTree(aPayKey.PubKey)
	require.NoError(t.t, err)

	bCommitmentAllocation := []tapchannel.Allocation{
		{
			// Commitment transaction for initiator (B).
			OutputIndex: 0,
			InternalKey: bToLocalTree.InternalKey,
			TapscriptSibling: commitment.NewPreimageFromBranch(
				txscript.NewTapBranch(
					bToLocalTreeRoot.Left(),
					bToLocalTreeRoot.Right(),
				),
			),
			ScriptKey: asset.NewScriptKey(
				bToLocalTree.TaprootKey,
			),
			AssetVersion:    asset.V1,
			Amount:          assetFundingAmountB,
			BtcAmount:       btcFundingAmountB,
			BtcSubtractFees: true,
		},
		{
			// Commitment transaction for responder (A).
			OutputIndex: 1,
			InternalKey: bToRemoteTree.InternalKey,
			TapscriptSibling: commitment.NewPreimageFromLeaf(
				bToRemoteTree.SettleLeaf,
			),
			ScriptKey: asset.NewScriptKey(
				bToRemoteTree.TaprootKey,
			),
			AssetVersion: asset.V1,
			Amount:       assetFundingAmountA,
			BtcAmount:    tapscript.DummyAmtSats,
		},
	}
	commitmentVirtualPackets, err := tapchannel.DistributeCoins(
		ctxt, fn.Values(fundingInputProofs),
		bCommitmentAllocation, assetIDX, assetIDY, assetIDZ,
		chainParams,
	)
	require.NoError(t.t, err)

	commitmentPacket, err := tapfreighter.CreateFundedAnchorTX(
		bCommitmentAllocation, commitmentVirtualPackets, nil, feeRate,
	)
	require.NoError(t.t, err)

	commitmentPsbt := commitmentPacket.FundedPsbt.Pkt
	commitmentPacketB64, err := commitmentPsbt.B64Encode()
	require.NoError(t.t, err)

	t.Logf("BTC level commitment packet: %v\n", commitmentPacketB64)

	// With the commitment transaction created, we can now create partial
	// sigs for it, first on the asset level then on the BTC level. We start
	// with the partial sigs of the initiator to arrive at the full
	// funding_created message.
	var (
		bTapPartialSigsWithNonces = make(map[asset.ID][]byte)
		bTapSessIDX               []byte
		bTapSessIDY               []byte
		bTapSessIDZ               []byte
	)
	bTapPartialSigsWithNonces[assetIDX], bTapSessIDX = tapCreatePartialSig(
		t.t, B, selectPacket(t.t, commitmentVirtualPackets, assetIDX),
		bFundingKey, bTapNoncesX, aFundingKey.PubKey,
		aTapNoncesX.PubNonce,
	)
	bTapPartialSigsWithNonces[assetIDY], bTapSessIDY = tapCreatePartialSig(
		t.t, B, selectPacket(t.t, commitmentVirtualPackets, assetIDY),
		bFundingKey, bTapNoncesY, aFundingKey.PubKey,
		aTapNoncesY.PubNonce,
	)
	bTapPartialSigsWithNonces[assetIDZ], bTapSessIDZ = tapCreatePartialSig(
		t.t, B, selectPacket(t.t, commitmentVirtualPackets, assetIDZ),
		bFundingKey, bTapNoncesZ, aFundingKey.PubKey,
		aTapNoncesZ.PubNonce,
	)

	bPartialSignatureWithNonce, bSessID := createPartialSig(
		t.t, lndB, commitmentPacket.FundedPsbt.Pkt, bFundingKey,
		bBtcNonces, aFundingKey.PubKey, aBtcNonces.PubNonce,
	)

	fundingCreatedMsg := &fundingCreated{
		tempChannelID: tempChanID,
		fundingTxid: fn.ByteSlice(
			fundingPacket.FundedPsbt.Pkt.UnsignedTx.TxHash(),
		),
		fundingOutput:                 0,
		partialSignatureWithNonce:     bPartialSignatureWithNonce,
		tapPartialSignaturesWithNonce: bTapPartialSigsWithNonces,
	}
	t.Logf("funding_created: %#v\n", fundingCreatedMsg)

	// And now to be able to respond with the full funding_signed message,
	// we need to create partial sigs for the responder.
	aTapPartialSigsWithNonces := make(map[asset.ID][]byte)
	aTapPartialSigsWithNonces[assetIDX], _ = tapCreatePartialSig(
		t.t, A, selectPacket(t.t, commitmentVirtualPackets, assetIDX),
		aFundingKey, aTapNoncesX, bFundingKey.PubKey,
		bTapNoncesX.PubNonce,
	)
	aTapPartialSigsWithNonces[assetIDY], _ = tapCreatePartialSig(
		t.t, A, selectPacket(t.t, commitmentVirtualPackets, assetIDY),
		aFundingKey, aTapNoncesY, bFundingKey.PubKey,
		bTapNoncesY.PubNonce,
	)
	aTapPartialSigsWithNonces[assetIDZ], _ = tapCreatePartialSig(
		t.t, A, selectPacket(t.t, commitmentVirtualPackets, assetIDZ),
		aFundingKey, aTapNoncesZ, bFundingKey.PubKey,
		bTapNoncesZ.PubNonce,
	)

	aPartialSignatureWithNonce, _ := createPartialSig(
		t.t, lndA, commitmentPacket.FundedPsbt.Pkt, aFundingKey,
		aBtcNonces, bFundingKey.PubKey, bBtcNonces.PubNonce,
	)

	fundingSignedMsg := &fundingSigned{
		channelID:                     tempChanID,
		partialSignatureWithNonce:     aPartialSignatureWithNonce,
		tapPartialSignaturesWithNonce: aTapPartialSigsWithNonces,
	}
	t.Logf("funding_signed: %#v\n", fundingSignedMsg)

	// And now comes the last step of the funding phase, we create the fully
	// signed funding transaction. We need to cheat a little bit here since
	// we basically do a dual funding, which isn't supported on the protocol
	// level. So we need to manually sign the asset carrying inputs by the
	// two parties. Since we have the full derivation information and
	// Taproot merkle roots available in the PSBT, we can just pass in the
	// same PSBT into both lnd nodes, and they will each sign the asset
	// inputs they know the key for. And finally we'll finalize the PSBT at
	// B since they provide the BTC-only funding input.
	fundingPsbt := signPsbt(t.t, lndA, fundingPacket.FundedPsbt.Pkt)
	fundingPsbt = signPsbt(t.t, lndB, fundingPsbt)
	fundingTx := finalizeAndPublish(t.t, lndB, fundingPsbt)

	t.Logf("Funding transaction published: %v\n", fundingTx.TxHash())

	// Combine the partial signatures for the commitment transaction now and
	// then publish it.
	finalBtcCommitmentWitness := combineSigs(
		t.t, lndB, bSessID, aPartialSignatureWithNonce,
	)

	var witnessBuf bytes.Buffer
	err = psbt.WriteTxWitness(&witnessBuf, finalBtcCommitmentWitness)
	require.NoError(t.t, err)

	commitmentPsbt.Inputs[0].FinalScriptWitness = witnessBuf.Bytes()
	commitmentTx := finalizeAndPublish(t.t, lndB, commitmentPsbt)

	t.Logf("Commitment transaction published: %v\n", commitmentTx.TxHash())

	// Let's create and validate the commitment proofs for the initiator
	// commitment output (index 0 on the commitment transaction). Since
	// the balance didn't change, the initiator (A) only has assets Y and Z.
	finalTapCommitmentWitnessY := combineSigs(
		t.t, lndB, bTapSessIDY, aTapPartialSigsWithNonces[assetIDY],
	)
	commitmentProofY := createProof(
		t.t, commitmentPacket,
		selectPacket(t.t, commitmentVirtualPackets, assetIDY), 0,
		finalTapCommitmentWitnessY, commitmentVirtualPackets,
	)
	commitmentProofFileY := combineProofs(
		t.t, proofFileY, fundingInputProofs[assetIDY], commitmentProofY,
	)
	snapshotY, err := commitmentProofFileY.Verify(
		ctxt, ignoreHeaderVerifier, ignoreGroupVerifier,
	)
	require.NoError(t.t, err)
	t.Logf("Commitment output proof Y verified for script key %x",
		snapshotY.Asset.ScriptKey.PubKey.SerializeCompressed())

	finalTapCommitmentWitnessZ := combineSigs(
		t.t, lndB, bTapSessIDZ, aTapPartialSigsWithNonces[assetIDZ],
	)
	commitmentProofZ := createProof(
		t.t, commitmentPacket,
		selectPacket(t.t, commitmentVirtualPackets, assetIDZ), 0,
		finalTapCommitmentWitnessZ, commitmentVirtualPackets,
	)
	commitmentProofFileZ := combineProofs(
		t.t, proofFileZ, fundingInputProofs[assetIDZ], commitmentProofZ,
	)
	snapshotZ, err := commitmentProofFileZ.Verify(
		ctxt, ignoreHeaderVerifier, ignoreGroupVerifier,
	)
	require.NoError(t.t, err)
	t.Logf("Commitment output proof Z verified for script key %x",
		snapshotZ.Asset.ScriptKey.PubKey.SerializeCompressed())

	finalTapCommitmentWitnessX := combineSigs(
		t.t, lndB, bTapSessIDX, aTapPartialSigsWithNonces[assetIDX],
	)
	t.Logf("Witnesses: %x, %x, %x\n", finalTapCommitmentWitnessX,
		finalTapCommitmentWitnessY, finalTapCommitmentWitnessZ)
}

func sendAssetsAndAssert(t *harnessTest, from, to *tapdHarness, a *taprpc.Asset,
	amount uint64, outNumTransfer, inNumTransfer int) {

	ctxb := context.Background()
	ctxt, cancel := context.WithTimeout(ctxb, defaultWaitTimeout)
	defer cancel()

	addr, err := to.NewAddr(ctxt, &taprpc.NewAddrRequest{
		AssetId: a.AssetGenesis.AssetId,
		Amt:     amount,
	})
	require.NoError(t.t, err)

	sendResp := sendAssetsToAddr(t, from, addr)

	total := a.Amount
	ConfirmAndAssertOutboundTransfer(
		t.t, t.lndHarness.Miner.Client, from, sendResp,
		a.AssetGenesis.AssetId, []uint64{total - amount, amount},
		outNumTransfer-1, outNumTransfer,
	)

	AssertNonInteractiveRecvComplete(t.t, to, inNumTransfer)
}

func findAsset(assets []*taprpc.Asset, assetID []byte) *taprpc.Asset {
	for _, a := range assets {
		if bytes.Equal(a.AssetGenesis.AssetId, assetID) {
			return a
		}
	}

	return nil
}

func fetchProofFile(t *testing.T, src *tapdHarness,
	a *taprpc.Asset) *proof.File {

	ctxb := context.Background()
	ctxt, cancel := context.WithTimeout(ctxb, defaultWaitTimeout)
	defer cancel()

	resp, err := src.ExportProof(ctxt, &taprpc.ExportProofRequest{
		AssetId:   a.AssetGenesis.AssetId,
		ScriptKey: a.ScriptKey,
	})
	require.NoError(t, err)

	f := &proof.File{}
	err = f.Decode(bytes.NewReader(resp.RawProofFile))
	require.NoError(t, err)

	return f
}

func extractLastProof(t *testing.T, f *proof.File) *proof.Proof {
	lastProof, err := f.LastProof()
	require.NoError(t, err)

	return lastProof
}

func serializeProof(t *testing.T, p *proof.Proof) []byte {
	var buf bytes.Buffer
	err := p.Encode(&buf)
	require.NoError(t, err)

	return buf.Bytes()
}

func assetRoot(t *testing.T,
	rpcAssets []*taprpc.Asset) *universerpc.MerkleSumNode {

	assets := fn.Map(rpcAssets, func(a *taprpc.Asset) *asset.Asset {
		return assetFromRPC(t, a)
	})

	ac, err := commitment.NewAssetCommitment(assets...)
	require.NoError(t, err)

	tc, err := commitment.NewTapCommitment(ac)
	require.NoError(t, err)

	return &universerpc.MerkleSumNode{
		RootHash: fn.ByteSlice(tc.TreeRoot.NodeHash()),
		RootSum:  int64(tc.TreeRoot.NodeSum()),
	}
}

func assetFromRPC(t *testing.T, rpcAsset *taprpc.Asset) *asset.Asset {
	genPoint, err := ParseOutPoint(rpcAsset.AssetGenesis.GenesisPoint)
	require.NoError(t, err)

	gen := asset.Genesis{
		FirstPrevOut: *genPoint,
		Tag:          rpcAsset.AssetGenesis.Name,
		MetaHash: fn.ToArray[[32]byte](
			rpcAsset.AssetGenesis.MetaHash,
		),
		OutputIndex: rpcAsset.AssetGenesis.OutputIndex,
		Type:        asset.Type(rpcAsset.AssetGenesis.AssetType),
	}

	scriptPubKey, err := btcec.ParsePubKey(rpcAsset.ScriptKey)
	require.NoError(t, err)

	var groupKey *asset.GroupKey
	if rpcAsset.AssetGroup != nil {
		rawGroupKey, err := btcec.ParsePubKey(
			rpcAsset.AssetGroup.RawGroupKey,
		)
		require.NoError(t, err)

		groupPubKey, err := btcec.ParsePubKey(
			rpcAsset.AssetGroup.TweakedGroupKey,
		)
		require.NoError(t, err)

		groupKey = &asset.GroupKey{
			RawKey: keychain.KeyDescriptor{
				PubKey: rawGroupKey,
			},
			GroupPubKey: *groupPubKey,
		}
	}

	a, err := asset.New(
		gen, rpcAsset.Amount, uint64(rpcAsset.LockTime),
		uint64(rpcAsset.RelativeLockTime),
		asset.NewScriptKey(scriptPubKey), groupKey,
	)
	require.NoError(t, err)

	return a
}

func deriveKey(t *testing.T, lnd *node.HarnessNode,
	family keychain.KeyFamily) keychain.KeyDescriptor {

	ctxb := context.Background()
	ctxt, cancel := context.WithTimeout(ctxb, defaultWaitTimeout)
	defer cancel()

	nextKey, err := lnd.RPC.WalletKit.DeriveNextKey(ctxt, &walletrpc.KeyReq{
		KeyFamily: int32(family),
	})
	require.NoError(t, err)

	desc, err := unmarshalKeyDescriptor(nextKey)
	require.NoError(t, err)

	return desc
}

// unmarshalKeyDescriptor parses the RPC key descriptor into the native
// counterpart.
func unmarshalKeyDescriptor(
	rpcDesc *signrpc.KeyDescriptor) (keychain.KeyDescriptor, error) {

	var (
		desc keychain.KeyDescriptor
		err  error
	)

	// The public key of a key descriptor is mandatory. It is enough to
	// locate the corresponding private key in the backing wallet. But to
	// speed things up (and for additional context), the locator should
	// still be provided if available.
	desc.PubKey, err = btcec.ParsePubKey(rpcDesc.RawKeyBytes)
	if err != nil {
		return desc, err
	}

	if rpcDesc.KeyLoc != nil {
		desc.KeyLocator = keychain.KeyLocator{
			Family: keychain.KeyFamily(rpcDesc.KeyLoc.KeyFamily),
			Index:  uint32(rpcDesc.KeyLoc.KeyIndex),
		}
	}

	return desc, nil
}

func pubKeyBytes(k keychain.KeyDescriptor) []byte {
	return k.PubKey.SerializeCompressed()
}

func createRevocationProducer(t *testing.T, lnd *node.HarnessNode,
	revKey keychain.KeyDescriptor,
	fundingKey *btcec.PublicKey) shachain.Producer {

	ctxb := context.Background()
	ctxt, cancel := context.WithTimeout(ctxb, defaultWaitTimeout)
	defer cancel()

	resp, err := lnd.RPC.Signer.DeriveSharedKey(
		ctxt, &signrpc.SharedKeyRequest{
			KeyDesc: &signrpc.KeyDescriptor{
				RawKeyBytes: pubKeyBytes(revKey),
				KeyLoc: &signrpc.KeyLocator{
					KeyFamily: int32(revKey.Family),
					KeyIndex:  int32(revKey.Index),
				},
			},
			EphemeralPubkey: fundingKey.SerializeCompressed(),
		},
	)
	require.NoError(t, err)

	var revRoot [32]byte
	copy(revRoot[:], resp.SharedKey)

	return shachain.NewRevocationProducer(revRoot)
}

func commitPointAt(t *testing.T, producer shachain.Producer,
	index uint64) *btcec.PublicKey {

	revocationSecret, err := producer.AtIndex(index)
	require.NoError(t, err)

	return input.ComputeCommitmentPoint(revocationSecret[:])
}

func getUtxo(t *testing.T, lnd *node.HarnessNode,
	minSize int64) *chanfunding.Coin {

	ctxb := context.Background()
	ctxt, cancel := context.WithTimeout(ctxb, defaultWaitTimeout)
	defer cancel()

	resp, err := lnd.RPC.WalletKit.ListUnspent(
		ctxt, &walletrpc.ListUnspentRequest{
			MinConfs: 1,
		},
	)
	require.NoError(t, err)

	for _, utxo := range resp.Utxos {
		if utxo.AmountSat < minSize {
			continue
		}

		outPoint, err := walletrpc.UnmarshallOutPoint(utxo.Outpoint)
		require.NoError(t, err)

		pkScriptBytes, err := hex.DecodeString(utxo.PkScript)
		require.NoError(t, err)

		return &chanfunding.Coin{
			TxOut: wire.TxOut{
				Value:    utxo.AmountSat,
				PkScript: pkScriptBytes,
			},
			OutPoint: *outPoint,
		}
	}

	t.Fatalf("no UTXO found with min size %v", minSize)
	return nil
}

func selectPacket(t *testing.T, vPackets []*tappsbt.VPacket,
	assetID asset.ID) *tappsbt.VPacket {

	vPkt, err := fn.First(vPackets, func(p *tappsbt.VPacket) bool {
		return p.Inputs[0].PrevID.ID == assetID
	})
	require.NoError(t, err)

	return vPkt
}

func addAnchorDerivationInfo(t *testing.T, tapd *tapdHarness,
	pkt *psbt.Packet) {

	ctxb := context.Background()
	ctxt, cancel := context.WithTimeout(ctxb, defaultWaitTimeout)
	defer cancel()

	notFoundErr := address.ErrInternalKeyNotFound.Error()

	for idx := range pkt.Inputs {
		pIn := &pkt.Inputs[idx]

		// We only care about asset inputs.
		if len(pIn.TaprootMerkleRoot) == 0 {
			continue
		}

		// We can't query the internal key if there is none specified.
		if len(pIn.TaprootInternalKey) != 32 {
			continue
		}

		// If we already have the derivation info, we can skip this
		// input.
		if len(pIn.TaprootBip32Derivation) > 0 {
			continue
		}

		// Let's query our node for the internal key information now.
		resp, err := tapd.QueryInternalKey(
			ctxt, &wrpc.QueryInternalKeyRequest{
				InternalKey: pIn.TaprootInternalKey,
			},
		)
		if err != nil && strings.Contains(err.Error(), notFoundErr) {
			// If the internal key is not known, we can't add the
			// derivation info.
			continue
		}
		require.NoError(t, err)

		keyDesc, err := tap.UnmarshalKeyDescriptor(resp.InternalKey)
		require.NoError(t, err)

		derivation, trDerivation := tappsbt.Bip32DerivationFromKeyDesc(
			keyDesc, tapd.cfg.NetParams.HDCoinType,
		)
		pIn.Bip32Derivation = []*psbt.Bip32Derivation{derivation}
		pIn.TaprootBip32Derivation = []*psbt.TaprootBip32Derivation{
			trDerivation,
		}
	}
}

func signVirtualPsbt(t *testing.T, tapd *tapdHarness,
	vPacket *tappsbt.VPacket) *tappsbt.VPacket {

	ctxb := context.Background()
	ctxt, cancel := context.WithTimeout(ctxb, defaultWaitTimeout)
	defer cancel()

	var buf bytes.Buffer
	err := vPacket.Serialize(&buf)
	require.NoError(t, err)

	resp, err := tapd.SignVirtualPsbt(ctxt, &wrpc.SignVirtualPsbtRequest{
		FundedPsbt: buf.Bytes(),
	})
	require.NoError(t, err)

	require.GreaterOrEqual(t, len(resp.SignedInputs), 1)

	result, err := tappsbt.NewFromRawBytes(
		bytes.NewReader(resp.SignedPsbt), false,
	)
	require.NoError(t, err)

	return result
}

func tapCreatePartialSig(t *testing.T, tapd *tapdHarness, vPkt *tappsbt.VPacket,
	localKey keychain.KeyDescriptor, localNonces *musig2.Nonces,
	otherKey *btcec.PublicKey,
	otherNonces [musig2.PubNonceSize]byte) ([]byte, []byte) {

	lnd := tapd.cfg.LndNode
	sessID := tapMuSig2Session(
		t, lnd, localKey, otherKey.SerializeCompressed(), *localNonces,
		[][]byte{otherNonces[:]},
	)

	partialSigner := &muSig2PartialSigner{
		sessID: sessID,
		lnd:    lnd,
	}

	// The signing code requires us to specify the BIP-0032 derivation info
	// for the key we want to sign with. We can't do that because it's a
	// combined key. But since our integration test partial signer will just
	// ignore the key anyway, we simply provide a fake key to bypass the
	// check.
	// TODO(guggero): Make this nicer by implementing the proposed MuSig2
	// fields for PSBT.
	vIn := vPkt.Inputs[0]
	derivation, trDerivation := tappsbt.Bip32DerivationFromKeyDesc(
		keychain.KeyDescriptor{
			PubKey: localKey.PubKey,
		}, tapd.cfg.NetParams.HDCoinType,
	)
	vIn.Bip32Derivation = []*psbt.Bip32Derivation{derivation}
	vIn.TaprootBip32Derivation = []*psbt.TaprootBip32Derivation{
		trDerivation,
	}

	err := tapscript.SignVirtualTransaction(
		vPkt, partialSigner, partialSigner,
	)
	require.NoError(t, err)

	isSplit, err := vPkt.HasSplitCommitment()
	require.NoError(t, err)

	// Identify new output asset. For splits, the new asset that received
	// the signature is the one with the split root set to true.
	newAsset := vPkt.Outputs[0].Asset
	if isSplit {
		splitOut, err := vPkt.SplitRootOutput()
		require.NoError(t, err)

		newAsset = splitOut.Asset
	}

	// The first part of the witness is just a fake R value, which we can
	// ignore.
	partialSig := newAsset.PrevWitnesses[0].TxWitness[0][32:]

	return partialSig, sessID
}

type muSig2PartialSigner struct {
	sessID []byte
	lnd    *node.HarnessNode
}

func (m *muSig2PartialSigner) SignVirtualTx(_ *lndclient.SignDescriptor,
	tx *wire.MsgTx, prevOut *wire.TxOut) (*schnorr.Signature, error) {

	prevOutputFetcher := txscript.NewCannedPrevOutputFetcher(
		prevOut.PkScript, prevOut.Value,
	)
	sighashes := txscript.NewTxSigHashes(tx, prevOutputFetcher)

	sigHash, err := txscript.CalcTaprootSignatureHash(
		sighashes, txscript.SigHashDefault, tx, 0, prevOutputFetcher,
	)
	if err != nil {
		return nil, err
	}

	ctxb := context.Background()
	ctxt, cancel := context.WithTimeout(ctxb, defaultWaitTimeout)
	defer cancel()

	sign, err := m.lnd.RPC.Signer.MuSig2Sign(
		ctxt, &signrpc.MuSig2SignRequest{
			SessionId:     m.sessID,
			MessageDigest: sigHash,
			Cleanup:       false,
		},
	)
	if err != nil {
		return nil, err
	}

	// We only get the 32-byte partial signature (just the s value).
	// So we just use an all-zero value for R, since the parsing mechanism
	// doesn't validate R to be a valid point on the curve.
	var sig [schnorr.SignatureSize]byte
	copy(sig[32:], sign.LocalPartialSignature)

	return schnorr.ParseSignature(sig[:])
}

func (m *muSig2PartialSigner) Execute(*asset.Asset, []*commitment.SplitAsset,
	commitment.InputSet) error {

	return nil
}

func tapMuSig2Session(t *testing.T, lnd *node.HarnessNode,
	localKey keychain.KeyDescriptor, otherKey []byte,
	localNonces musig2.Nonces, otherNonces [][]byte) []byte {

	ctxb := context.Background()
	ctxt, cancel := context.WithTimeout(ctxb, defaultWaitTimeout)
	defer cancel()

	version := signrpc.MuSig2Version_MUSIG2_VERSION_V100RC2
	sess, err := lnd.RPC.Signer.MuSig2CreateSession(
		ctxt, &signrpc.MuSig2SessionRequest{
			KeyLoc: &signrpc.KeyLocator{
				KeyFamily: int32(localKey.Family),
				KeyIndex:  int32(localKey.Index),
			},
			AllSignerPubkeys: [][]byte{
				localKey.PubKey.SerializeCompressed(),
				otherKey,
			},
			OtherSignerPublicNonces: otherNonces,
			TaprootTweak: &signrpc.TaprootTweakDesc{
				KeySpendOnly: true,
			},
			Version:                version,
			PregeneratedLocalNonce: localNonces.SecNonce[:],
		},
	)
	require.NoError(t, err)

	return sess.SessionId
}

func createPartialSig(t *testing.T, lnd *node.HarnessNode, pkt *psbt.Packet,
	localKey keychain.KeyDescriptor, localNonces *musig2.Nonces,
	otherKey *btcec.PublicKey,
	otherNonces [musig2.PubNonceSize]byte) ([]byte, []byte) {

	scriptRoot := pkt.Inputs[0].TaprootMerkleRoot

	sessID := muSig2Session(
		t, lnd, scriptRoot, localKey, otherKey.SerializeCompressed(),
		*localNonces, [][]byte{otherNonces[:]},
	)

	partialSigner := &muSig2PartialSigner{
		sessID: sessID,
		lnd:    lnd,
	}
	sig, err := partialSigner.SignVirtualTx(
		nil, pkt.UnsignedTx, pkt.Inputs[0].WitnessUtxo,
	)
	require.NoError(t, err)

	return sig.Serialize()[32:], sessID
}

func muSig2Session(t *testing.T, lnd *node.HarnessNode, scriptRoot []byte,
	localKey keychain.KeyDescriptor, otherKey []byte,
	localNonces musig2.Nonces, otherNonces [][]byte) []byte {

	ctxb := context.Background()
	ctxt, cancel := context.WithTimeout(ctxb, defaultWaitTimeout)
	defer cancel()

	version := signrpc.MuSig2Version_MUSIG2_VERSION_V100RC2
	sess, err := lnd.RPC.Signer.MuSig2CreateSession(
		ctxt, &signrpc.MuSig2SessionRequest{
			KeyLoc: &signrpc.KeyLocator{
				KeyFamily: int32(localKey.Family),
				KeyIndex:  int32(localKey.Index),
			},
			AllSignerPubkeys: [][]byte{
				localKey.PubKey.SerializeCompressed(),
				otherKey,
			},
			OtherSignerPublicNonces: otherNonces,
			TaprootTweak: &signrpc.TaprootTweakDesc{
				ScriptRoot: scriptRoot,
			},
			Version:                version,
			PregeneratedLocalNonce: localNonces.SecNonce[:],
		},
	)
	require.NoError(t, err)

	return sess.SessionId
}

func signPsbt(t *testing.T, lnd *node.HarnessNode,
	pkt *psbt.Packet) *psbt.Packet {

	ctxb := context.Background()
	ctxt, cancel := context.WithTimeout(ctxb, defaultWaitTimeout)
	defer cancel()

	var buf bytes.Buffer
	err := pkt.Serialize(&buf)
	require.NoError(t, err)

	resp, err := lnd.RPC.WalletKit.SignPsbt(
		ctxt, &walletrpc.SignPsbtRequest{
			FundedPsbt: buf.Bytes(),
		},
	)
	require.NoError(t, err)

	result, err := psbt.NewFromRawBytes(
		bytes.NewReader(resp.SignedPsbt), false,
	)
	require.NoError(t, err)

	// Try to finalize the input(s) we just signed.
	for _, signedIndex := range resp.SignedInputs {
		ok, err := psbt.MaybeFinalize(result, int(signedIndex))
		require.NoError(t, err)

		require.True(t, ok)
	}

	return result
}

func finalizeAndPublish(t *testing.T, lnd *node.HarnessNode,
	pkt *psbt.Packet) *wire.MsgTx {

	ctxb := context.Background()
	ctxt, cancel := context.WithTimeout(ctxb, defaultWaitTimeout)
	defer cancel()

	var (
		buf          bytes.Buffer
		finalTxBytes []byte
		finalTx      = &wire.MsgTx{}
	)
	if !pkt.IsComplete() {
		err := pkt.Serialize(&buf)
		require.NoError(t, err)

		finalizeResp := lnd.RPC.FinalizePsbt(
			&walletrpc.FinalizePsbtRequest{
				FundedPsbt: buf.Bytes(),
			},
		)

		_, err = psbt.NewFromRawBytes(
			bytes.NewReader(finalizeResp.SignedPsbt), false,
		)
		require.NoError(t, err)

		err = finalTx.Deserialize(
			bytes.NewReader(finalizeResp.RawFinalTx),
		)
		require.NoError(t, err)

		finalTxBytes = finalizeResp.RawFinalTx
	} else {
		var err error
		finalTx, err = psbt.Extract(pkt)
		require.NoError(t, err)

		err = finalTx.Serialize(&buf)
		require.NoError(t, err)

		finalTxBytes = buf.Bytes()
	}

	t.Logf("Publishing transaction %v: %s\n", finalTx.TxHash(),
		spew.Sdump(finalTx))

	resp, err := lnd.RPC.WalletKit.PublishTransaction(
		ctxt, &walletrpc.Transaction{
			TxHex: finalTxBytes,
		},
	)
	require.NoError(t, err)
	require.Empty(t, resp.PublishError)

	return finalTx
}

func combineSigs(t *testing.T, lnd *node.HarnessNode, sessID,
	otherPartialSig []byte) wire.TxWitness {

	ctxb := context.Background()
	ctxt, cancel := context.WithTimeout(ctxb, defaultWaitTimeout)
	defer cancel()

	resp, err := lnd.RPC.Signer.MuSig2CombineSig(
		ctxt, &signrpc.MuSig2CombineSigRequest{
			SessionId:              sessID,
			OtherPartialSignatures: [][]byte{otherPartialSig},
		},
	)
	require.NoError(t, err)
	require.True(t, resp.HaveAllSignatures)

	commitmentWitness := make(wire.TxWitness, 1)
	commitmentWitness[0] = resp.FinalSignature

	return commitmentWitness
}

func createProof(t *testing.T, anchorTx *tapfreighter.AnchorTransaction,
	vPkt *tappsbt.VPacket, vPktOutputIndex int, witness wire.TxWitness,
	allAnchoredVPackets []*tappsbt.VPacket) *proof.Proof {

	splitRoot := vPkt.Outputs[0].Asset
	if vPkt.HasSplitRootOutput() {
		splitRootOut, err := vPkt.SplitRootOutput()
		require.NoError(t, err)

		splitRoot = splitRootOut.Asset
	}

	splitRoot.PrevWitnesses[0].TxWitness = witness

	proofSuffix, err := tapfreighter.CreateProofSuffix(
		anchorTx, vPkt, vPktOutputIndex, allAnchoredVPackets,
	)
	require.NoError(t, err)

	return proofSuffix
}

func combineProofs(t *testing.T, f *proof.File,
	proofs ...*proof.Proof) *proof.File {

	for _, p := range proofs {
		err := f.AppendProof(*p)
		require.NoError(t, err)
	}

	return f
}

func ignoreHeaderVerifier(wire.BlockHeader, uint32) error {
	return nil
}

func ignoreGroupVerifier(*btcec.PublicKey) error {
	return nil
}
