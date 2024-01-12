package tapscript

import (
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/waddrmgr"
	"github.com/btcsuite/btcwallet/wallet/txsizes"
	"github.com/lightninglabs/taproot-assets/commitment"
	"github.com/lightninglabs/taproot-assets/fn"
	"github.com/lightningnetwork/lnd/lnwallet/chainfee"
)

// PayToAddrScript constructs a P2TR script that embeds a Taproot Asset
// commitment by tweaking the receiver key by a Tapscript tree that contains the
// Taproot Asset commitment root. The Taproot Asset commitment must be
// reconstructed by the receiver, and they also need to Tapscript sibling hash
// used here if present.
func PayToAddrScript(internalKey btcec.PublicKey, sibling *chainhash.Hash,
	commitment commitment.TapCommitment) ([]byte, error) {

	tapscriptRoot := commitment.TapscriptRoot(sibling)
	outputKey := txscript.ComputeTaprootOutputKey(
		&internalKey, tapscriptRoot[:],
	)

	return PayToTaprootScript(outputKey)
}

// PayToTaprootScript creates a pk script for a pay-to-taproot output key.
func PayToTaprootScript(taprootKey *btcec.PublicKey) ([]byte, error) {
	return txscript.NewScriptBuilder().
		AddOp(txscript.OP_1).
		AddData(schnorr.SerializePubKey(taprootKey)).
		Script()
}

// TapTreeToSibling constucts a taproot sibling hash from a Tapscript
// tree, to be used with a TapCommitment tree root to derive a tapscript root.
func TapTreeToSibling(tapTree waddrmgr.Tapscript) (*chainhash.Hash,
	error) {

	var tapLeafHash *chainhash.Hash
	switch tapTree.Type {
	case waddrmgr.TapscriptTypeFullTree:
		// TODO(jhb): Is this the right way to handle a tree?
		switch len(tapTree.Leaves) {
		case 1:
			// A single tapscript leaf must be verified to not be
			// another Taproot Asset commitment before use.
			tapPreimage := commitment.NewPreimageFromLeaf(
				tapTree.Leaves[0],
			)
			err := tapPreimage.VerifyNoCommitment()
			if err != nil {
				return nil, err
			}

			tapLeafHash, err = tapPreimage.TapHash()
			if err != nil {
				return nil, err
			}

		default:
			tree := txscript.AssembleTaprootScriptTree(
				tapTree.Leaves...,
			)
			tapLeafHash = fn.Ptr(tree.RootNode.TapHash())
		}

	case waddrmgr.TapscriptTypePartialReveal:
		tapLeafHash = (*chainhash.Hash)(tapTree.ControlBlock.RootHash(
			tapTree.RevealedScript,
		))

	case waddrmgr.TaprootKeySpendRootHash:
		tapLeafHash = (*chainhash.Hash)(tapTree.RootHash)

	default:
		return nil, fmt.Errorf("invalid tapscript tree type: %v",
			tapTree.Type)
	}

	return tapLeafHash, nil
}

// EstimateFee provides a worst-case fee and vsize estimate for a transaction
// built from the given inputs and outputs. This mirrors the fee estimation
// implemented in btcwallet/wallet/txauthor/author.go:NewUnsignedTransaction()
// EstimateFee assumes that a change output (or a dummy output for change) is
// included in the set of given outputs.
func EstimateFee(inputScripts [][]byte, outputs []*wire.TxOut,
	feeRate chainfee.SatPerKWeight) (int, btcutil.Amount) {

	// Count the types of input scripts.
	var nested, p2wpkh, p2tr, p2pkh int
	for _, pkScript := range inputScripts {
		switch {
		// If this is a p2sh output, we assume this is a
		// nested P2WKH.
		case txscript.IsPayToScriptHash(pkScript):
			nested++
		case txscript.IsPayToWitnessPubKeyHash(pkScript):
			p2wpkh++
		case txscript.IsPayToTaproot(pkScript):
			p2tr++
		default:
			p2pkh++
		}
	}

	// The change output is already in the set of given outputs, so we don't
	// need to account for an additional output.
	maxSignedSize := txsizes.EstimateVirtualSize(
		p2pkh, p2tr, p2wpkh, nested, outputs, 0,
	)
	maxRequiredFee := feeRate.FeePerKVByte().FeeForVSize(
		int64(maxSignedSize),
	)

	return maxSignedSize, maxRequiredFee
}
