package main

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	preimage "github.com/ethereum-optimism/optimism/op-preimage"
	"github.com/ethereum-optimism/optimism/op-preimage/kvstore"
	oppio "github.com/ethereum-optimism/optimism/op-program/io"
	oplog "github.com/ethereum-optimism/optimism/op-service/log"
	"github.com/ethereum-optimism/optimism/op-service/opio"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"io"
	"io/fs"
	"os"
	"strings"
)

func main() {
	ctx := opio.WithInterruptBlocker(context.Background())
	ctx = opio.CancelOnInterrupt(ctx)

	logger := oplog.NewLogger(os.Stdout, oplog.CLIConfig{
		Level:  log.LvlInfo,
		Color:  false,
		Format: oplog.FormatTerminal,
	})
	logger.Info("started server")

	preimageChan := preimage.CreatePreimageChannel()
	hinterChan := preimage.CreateHinterChannel()
	err := server(ctx, logger, preimageChan, hinterChan)
	if err != nil {
		logger.Error("server error", "err", err)
		os.Exit(1)
	}
	log.Info("exited server")
	os.Exit(0)
}

func encodeU64(x uint64) []byte {
	return binary.BigEndian.AppendUint64(nil, x)
}

func server(ctx context.Context, logger log.Logger, preimageChannel oppio.FileChannel, hintChannel oppio.FileChannel) error {
	var serverDone chan error
	var hinterDone chan error
	defer func() {
		if err := preimageChannel.Close(); err != nil {
			logger.Error("preimage channel close err", "err", err)
		}
		if err := hintChannel.Close(); err != nil {
			logger.Error("hint channel close err", "err", err)
		}
	}()
	logger.Info("Starting server")
	kv := kvstore.NewMemKV()

	// prepare initial game
	s := uint64(1000)
	a := uint64(3)
	b := uint64(4)

	var diff []byte
	diff = append(diff, crypto.Keccak256(encodeU64(a))...)
	diff = append(diff, crypto.Keccak256(encodeU64(b))...)

	preHash := crypto.Keccak256Hash(encodeU64(s))
	diffHash := crypto.Keccak256Hash(diff)

	_ = kv.Put(preimage.LocalIndexKey(0).PreimageKey(), preHash[:])
	_ = kv.Put(preimage.LocalIndexKey(1).PreimageKey(), diffHash[:])
	_ = kv.Put(preimage.LocalIndexKey(2).PreimageKey(), encodeU64(s*a+b))

	getPreimage := preimage.PreimageGetter(func(key [32]byte) ([]byte, error) {
		return kv.Get(key)
	})

	hinter := preimage.HintHandler(func(hint string) error {
		parts := strings.Split(hint, " ")
		if len(parts) != 2 {
			return nil
		}
		p, err := hex.DecodeString(parts[1])
		if err != nil || len(p) != 32 {
			return nil
		}
		h := common.Hash(*(*[32]byte)(p))
		switch parts[0] {
		case "fetch-state":
			switch h {
			case preHash:
				log.Info("handling state fetch")
				_ = kv.Put(preimage.Keccak256Key(preHash).PreimageKey(), encodeU64(s))
			default:
				log.Warn("unknown state", "hash", h)
			}
		case "fetch-diff":
			switch h {
			case diffHash:
				log.Info("handling diff fetch")
				_ = kv.Put(preimage.Keccak256Key(diffHash).PreimageKey(), diff)
				_ = kv.Put(preimage.Keccak256Key(crypto.Keccak256Hash(encodeU64(a))).PreimageKey(), encodeU64(a))
				_ = kv.Put(preimage.Keccak256Key(crypto.Keccak256Hash(encodeU64(b))).PreimageKey(), encodeU64(b))
			default:
				log.Warn("unknown diff request", "hash", h)
			}
		default:
			log.Warn("unexpected hint", "hint", hint)
		}
		return nil
	})

	logger.Info("registering handlers")
	serverDone = handlePreimageRequests(logger, preimageChannel, getPreimage)
	hinterDone = routeHints(logger, hintChannel, hinter)
	select {
	case err := <-serverDone:
		return err
	case err := <-hinterDone:
		return err
	case <-ctx.Done():
		logger.Info("server got closing signal")
		return nil
	}
}

func routeHints(logger log.Logger, hHostRW io.ReadWriter, hinter preimage.HintHandler) chan error {
	chErr := make(chan error)
	hintReader := preimage.NewHintReader(hHostRW)
	go func() {
		defer close(chErr)
		for {
			if err := hintReader.NextHint(hinter); err != nil {
				if err == io.EOF || errors.Is(err, fs.ErrClosed) {
					logger.Debug("closing pre-image hint handler")
					return
				}
				logger.Error("pre-image hint router error", "err", err)
				chErr <- err
				return
			}
		}
	}()
	return chErr
}

func handlePreimageRequests(logger log.Logger, pHostRW io.ReadWriteCloser, getter preimage.PreimageGetter) chan error {
	chErr := make(chan error)
	server := preimage.NewOracleServer(pHostRW)
	go func() {
		defer close(chErr)
		for {
			if err := server.NextPreimageRequest(getter); err != nil {
				if err == io.EOF || errors.Is(err, fs.ErrClosed) {
					logger.Debug("closing pre-image server")
					return
				}
				logger.Error("pre-image server error", "error", err)
				chErr <- err
				return
			}
		}
	}()
	return chErr
}
