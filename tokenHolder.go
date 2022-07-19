package simplewebtools

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"hash"
	"log"
	"math/big"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type TokenHolder struct {
	m                            sync.Mutex
	tokens                       map[string]time.Time
	ctx                          context.Context
	cancel                       context.CancelFunc
	validExpiredDuration         int64
	tokenLength                  int64
	cleanupTicker                *time.Ticker
	tokenHolderCryptoReaderError bool
	hashImpl                     hash.Hash
	hashFunc                     func(string) string
}

func (t *TokenHolder) New() string {
	buf := make([]byte, atomic.LoadInt64(&t.tokenLength))
	var nt string
	for {
		if err := t.makeTokenCryptoRand(buf); err != nil {
			if !t.tokenHolderCryptoReaderError {
				log.Println("tokenholder crypto/reader error:", err.Error())
				t.tokenHolderCryptoReaderError = true
			}
			t.makeTokenRand(buf)
		}
		nt = string(buf)
		t.m.Lock()
		if _, ok := t.tokens[nt]; !ok {
			h := t.hashFunc(nt)
			t.tokens[h] = time.Now().Add(time.Duration(atomic.LoadInt64(&t.validExpiredDuration)))
			t.m.Unlock()
			return nt
		}
		t.m.Unlock()
	}
}

func (t *TokenHolder) makeTokenCryptoRand(buf []byte) error {
	for i := 0; i < len(buf); i++ {
		n, err := crand.Int(crand.Reader, big.NewInt(75))
		if err != nil {
			return err
		}
		buf[i] = byte(n.Int64() + 48)
	}
	return nil
}

func (t *TokenHolder) makeTokenRand(buf []byte) error {
	for i := 0; i < len(buf); i++ {
		buf[i] = byte(rand.Int31n(75) + 48)
	}
	return nil
}

func (t *TokenHolder) Drop(token string) {
	h := t.hashFunc(token)
	t.m.Lock()
	delete(t.tokens, h)
	t.m.Unlock()
}

func (t *TokenHolder) Checkout(token string) bool {
	h := t.hashFunc(token)
	t.m.Lock()
	v, ok := t.tokens[h]
	if ok {
		if v.Before(time.Now()) {
			delete(t.tokens, h)
			ok = false
		} else {
			t.tokens[h] = time.Now().Add(time.Duration(atomic.LoadInt64(&t.validExpiredDuration)))
		}
	}
	t.m.Unlock()
	return ok
}

func (t *TokenHolder) SetCleanupExpiredDuration(duration time.Duration) {
	t.m.Lock()
	if t.cleanupTicker == nil {
		t.cleanupTicker = time.NewTicker(duration)
	} else {
		t.cleanupTicker.Reset(duration)
	}
	t.m.Unlock()
}

func (t *TokenHolder) SetTokenLength(l int) {
	atomic.StoreInt64(&t.tokenLength, int64(l))
}

func (t *TokenHolder) SetValidExpiredDuration(duration time.Duration) {
	atomic.StoreInt64(&t.validExpiredDuration, int64(duration))
}

func (t *TokenHolder) Start(validTimeDuration time.Duration, checkTokenDuration time.Duration, tokenLength int, hash hash.Hash) {
	t.m.Lock()
	rand.Seed(time.Now().UnixNano() + int64(os.Getpid()))
	t.SetValidExpiredDuration(validTimeDuration)
	t.SetTokenLength(tokenLength)
	if t.cancel != nil {
		t.cancel()
	}
	t.ctx, t.cancel = context.WithCancel(context.Background())
	t.tokens = make(map[string]time.Time)
	if t.cleanupTicker == nil {
		t.cleanupTicker = time.NewTicker(checkTokenDuration)
	} else {
		t.cleanupTicker.Reset(checkTokenDuration)
	}
	t.hashImpl = hash
	t.hashFunc = func(token string) string {
		t.hashImpl.Reset()
		t.hashImpl.Write([]byte(token))
		return hex.EncodeToString(t.hashImpl.Sum(nil))
	}
	go func(ctx context.Context, ticker time.Ticker) { // cleanup expired tokens
		for {
			select {
			case <-ctx.Done():
				t.m.Lock()
				for ct := range t.tokens {
					delete(t.tokens, ct)
				}
				t.m.Unlock()
			case <-ticker.C:
				t.m.Lock()
				curr_time := time.Now()
				for ct, ctv := range t.tokens {
					if ctv.Before(curr_time) {
						delete(t.tokens, ct)
					}
				}
				t.m.Unlock()
			}
		}
	}(t.ctx, *t.cleanupTicker)
	t.m.Unlock()
}

func (t *TokenHolder) Stop() {
	t.m.Lock()
	t.cleanupTicker.Stop()
	if t.cancel != nil {
		t.cancel()
	}
	t.m.Unlock()
}

func (t *TokenHolder) CheckoutAndDrop(token string) bool {
	if t.Checkout(token) {
		t.Drop(token)
		return true
	}
	return false
}
