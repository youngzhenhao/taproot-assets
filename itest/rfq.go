package itest

import (
	"bytes"
	"math/rand"
	"time"

	"github.com/lightninglabs/taproot-assets/asset"
	"github.com/lightninglabs/taproot-assets/rfqmessages"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/stretchr/testify/require"
)

func testQuoteRequest(t *harnessTest) {
	t.lndHarness.EnsureConnected(t.lndHarness.Alice, t.lndHarness.Bob)

	//t.Logf("Connecting LND nodes alice and bob")
	//aliceInfo := t.lndHarness.Bob.RPC.GetInfo()
	//
	//req := &lnrpc.ConnectPeerRequest{
	//	Addr: &lnrpc.LightningAddress{
	//		Pubkey: aliceInfo.IdentityPubkey,
	//		Host:   t.lndHarness.Bob.Cfg.P2PAddr(),
	//	},
	//}
	//
	//ctxb := context.Background()
	//ctxt, cancel := context.WithTimeout(ctxb, defaultWaitTimeout)
	//defer cancel()
	//
	//_, err := t.lndHarness.Alice.RPC.LN.ConnectPeer(ctxt, req)
	//require.NoError(t.t, err, "unable to connect LND nodes alice and bob")

	//t.Logf("Send an RFQ quote request from Bob to Alice")
	//aliceInfo := t.lndHarness.Alice.RPC.GetInfo()
	//
	//aliceIdPubKey, err := hex.DecodeString(aliceInfo.IdentityPubkey)
	//require.NoError(t.t, err, "unable to decode bob's pubkey")

	// Generate a random quote request id.
	var randomQuoteRequestId [32]byte
	_, err := rand.Read(randomQuoteRequestId[:])
	require.NoError(t.t, err, "unable to generate random quote request id")

	// Generate a random asset id.
	var randomAssetId asset.ID
	_, err = rand.Read(randomAssetId[:])
	require.NoError(t.t, err, "unable to generate random asset id")

	quoteRequest := rfqmessages.QuoteRequest{
		ID:                randomQuoteRequestId,
		AssetID:           &randomAssetId,
		AssetAmount:       42,
		SuggestedRateTick: 10,
	}

	// TLV encode the quote request.
	var streamBuf bytes.Buffer
	err = quoteRequest.Encode(&streamBuf)
	require.NoError(t.t, err, "unable to encode quote request")
	quoteReqBytes := streamBuf.Bytes()

	//go func() {
	//	msgClient, cancel := t.lndHarness.Alice.RPC.SubscribeCustomMessages()
	//	defer cancel()
	//
	//	for {
	//		msg, err := msgClient.Recv()
	//		require.NoError(
	//			t.t, err, "custom message receive: %w", err,
	//		)
	//
	//		t.Logf("Received custom message: %v", msg)
	//	}
	//}()

	resAlice := t.lndHarness.Alice.RPC.GetInfo()
	t.Logf("Alice alias: %s", resAlice.Alias)

	resBob := t.lndHarness.Bob.RPC.GetInfo()
	t.Logf("Bob alias: %s", resBob.Alias)

	t.Logf("Sending custom message to alias: %s", resAlice.Alias)
	t.lndHarness.Bob.RPC.SendCustomMessage(&lnrpc.SendCustomMessageRequest{
		Peer: t.lndHarness.Alice.PubKey[:],
		Type: rfqmessages.MsgTypeQuoteRequest,
		Data: quoteReqBytes,
	})

	//t.Logf("Sending custom message to alias: %s", resBob.Alias)
	//t.lndHarness.Alice.RPC.SendCustomMessage(&lnrpc.SendCustomMessageRequest{
	//	Peer: t.lndHarness.Bob.PubKey[:],
	//	Type: rfqmessages.MsgTypeQuoteRequest,
	//	Data: quoteReqBytes,
	//})

	time.Sleep(20 * time.Second)
}
