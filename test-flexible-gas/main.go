package main

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/ethereum-optimism/optimism/op-e2e/e2eutils"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

const blockTime = time.Second

func main() {
	secrets, err := e2eutils.DefaultMnemonicConfig.Secrets()
	if err != nil {
		log.Fatalf("Failed to generate secrets: %v", err)
	}
	aliceAddr := crypto.PubkeyToAddress(secrets.Alice.PublicKey)
	bobAddr := crypto.PubkeyToAddress(secrets.Bob.PublicKey)

	// Connect to the Optimism RPC endpoint
	var client *ethclient.Client
	for {
		var err error
		client, err = ethclient.Dial("http://127.0.0.1:9545")
		if err == nil {
			break
		}
		log.Println("Failed to dial rpc endpoint, retrying...")
		time.Sleep(blockTime)
	}

	fmt.Println("before transfer")
	// transfer back and forth
	_, err = prettyTransfer(client, aliceAddr, bobAddr, secrets.Alice)
	if err != nil {
		log.Fatal("transfer alice -> bob", err)
	}
	_, err = prettyTransfer(client, bobAddr, aliceAddr, secrets.Bob)
	if err != nil {
		log.Fatal("transfer bob -> alice", err)
	}

	// todo deploy a contract

	// todo execute the contract
}

func prettyTransfer(client *ethclient.Client, from, to common.Address, fromPrivKey *ecdsa.PrivateKey) (*types.Receipt, error) {
	receipt, err := transfer(client, from, to, fromPrivKey)
	if err != nil {
		return nil, err
	}
	/*
		receiptJSON, err := json.MarshalIndent(receipt, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal indent: %v", err)
		}
		fmt.Println(string(receiptJSON))*/
	time.Sleep(blockTime) // make sure another block is created
	balanceBefore, err := client.BalanceAt(context.Background(), from, new(big.Int).SetUint64(receipt.BlockNumber.Uint64()-1))
	if err != nil {
		return nil, fmt.Errorf("balance at: %v", err)
	}
	balanceAfter, err := client.BalanceAt(context.Background(), from, new(big.Int).SetUint64(receipt.BlockNumber.Uint64()))
	if err != nil {
		return nil, fmt.Errorf("balance at: %v", err)
	}
	l1Cost := new(big.Int).Sub(balanceBefore, balanceAfter)
	intrinsicGas := big.NewInt(21_000)
	l1Cost.Sub(l1Cost, intrinsicGas.Mul(intrinsicGas, receipt.EffectiveGasPrice)) // intrinsic gas
	l1Cost.Sub(l1Cost, big.NewInt(100))                                           // transfer amount

	fmt.Println(l1Cost.Uint64())
	return receipt, nil
}

func transfer(client *ethclient.Client, from, to common.Address, fromPrivKey *ecdsa.PrivateKey) (*types.Receipt, error) {
	// transfer funds back and forth
	nonce, err := client.PendingNonceAt(context.Background(), from)
	if err != nil {
		return nil, fmt.Errorf("pending nonce at: %v", err)
	}
	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		return nil, fmt.Errorf("suggest gas price: %v", err)
	}
	tx := types.NewTransaction(nonce, to, big.NewInt(100), uint64(21_000), gasPrice, nil)
	chainID, err := client.ChainID(context.Background())
	if err != nil {
		return nil, fmt.Errorf("chain id: %v", err)
	}
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), fromPrivKey)
	if err != nil {
		return nil, fmt.Errorf("sign tx: %v", err)
	}

	// publish signed tx
	if err := client.SendTransaction(context.Background(), signedTx); err != nil {
		return nil, fmt.Errorf("send transaction: %v", err)
	}

	for {
		fmt.Println("checking if transaction added")
		receipt, err := client.TransactionReceipt(context.Background(), signedTx.Hash())
		if err == nil {
			return receipt, nil
		} else if err != nil && !errors.Is(err, ethereum.NotFound) {
			return nil, fmt.Errorf("transaction by hash: %v", err)
		}
		time.Sleep(blockTime)
	}
}
