// Copyright (c) 2024 Sidero Labs, Inc.
//
// Use of this software is governed by the Business Source License
// included in the LICENSE file.

package service

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

// DynamicCertificate is a certificate that can be reloaded from disk.
type DynamicCertificate struct {
	cert     tls.Certificate
	certFile string
	keyFile  string
	mu       sync.Mutex
	loaded   bool
}

// NewDynamicCertificate creates a new DynamicCertificate.
func NewDynamicCertificate(certFile, keyFile string) *DynamicCertificate {
	return &DynamicCertificate{
		certFile: certFile,
		keyFile:  keyFile,
	}
}

// Load the initial certificate.
func (c *DynamicCertificate) Load() error {
	cert, err := tls.LoadX509KeyPair(c.certFile, c.keyFile)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.loaded = true
	c.cert = cert

	return nil
}

// GetCertificate returns the current certificate.
//
// It is suitable for use with tls.Config.GetCertificate.
func (c *DynamicCertificate) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.loaded {
		return nil, fmt.Errorf("the cert wasn't loaded yet")
	}

	return &c.cert, nil
}

// Watch the certificate files for changes and reload them.
func (c *DynamicCertificate) Watch(ctx context.Context, logger *zap.Logger) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("error creating fsnotify watcher: %w", err)
	}
	defer w.Close() //nolint:errcheck

	if err = w.Add(c.certFile); err != nil {
		return fmt.Errorf("error adding watch for file %s: %w", c.certFile, err)
	}

	if err = w.Add(c.keyFile); err != nil {
		return fmt.Errorf("error adding watch for file %s: %w", c.keyFile, err)
	}

	handleEvent := func(e fsnotify.Event) error {
		defer func() {
			if err = c.Load(); err != nil {
				logger.Error("failed to load certs", zap.Error(err))

				return
			}

			logger.Info("reloaded certs")
		}()

		if !e.Has(fsnotify.Remove) && !e.Has(fsnotify.Rename) {
			return nil
		}

		if err = w.Remove(e.Name); err != nil {
			logger.Error("failed to remove file watch, it may have been deleted", zap.String("file", e.Name), zap.Error(err))
		}

		if err = w.Add(e.Name); err != nil {
			return fmt.Errorf("error adding watch for file %s: %w", e.Name, err)
		}

		return nil
	}

	for {
		select {
		case e := <-w.Events:
			if err = handleEvent(e); err != nil {
				return err
			}
		case err = <-w.Errors:
			return fmt.Errorf("received fsnotify error: %w", err)
		case <-ctx.Done():
			return nil
		}
	}
}

// WatchWithRestarts restarts the Watch on error.
func (c *DynamicCertificate) WatchWithRestarts(ctx context.Context, logger *zap.Logger) error {
	for {
		if err := c.Watch(ctx, logger); err != nil {
			logger.Error("watch error", zap.Error(err))
		} else {
			return nil
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(5 * time.Second): // retry
		}
	}
}
