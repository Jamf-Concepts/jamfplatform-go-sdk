// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptrace"
	"sync/atomic"
)

// sendState records whether the attempt currently in flight for one request got
// as far as having its request fully written to the connection. It is what lets
// the retry policy tell "nothing reached the server" from "the server may have
// acted on it", which a transport error alone cannot say.
//
// It travels in the request context rather than in the error. The error is the
// wrong carrier: http.Client replaces a transport error with its own value when
// Client.Timeout fires, dropping anything wrapped inside, and a caller-supplied
// client sets that timeout. The context survives every layer, and the retry
// policy is handed it.
//
// The flag is per attempt, not per request: sendTrackingTransport clears it as
// each attempt starts, so a retry that fails before sending is not judged by the
// attempt before it.
type sendState struct {
	method string
	sent   atomic.Bool
}

type sendStateKey struct{}

// withSendState seeds ctx with a fresh sendState for a request using method.
func withSendState(ctx context.Context, method string) (context.Context, *sendState) {
	st := &sendState{method: method}
	return context.WithValue(ctx, sendStateKey{}, st), st
}

// sendStateFrom returns the sendState seeded into ctx, or nil for a request that
// was not issued through doRequestFull or DoMultipart (a raw request on
// Transport.HTTPClient, say, or a token exchange).
func sendStateFrom(ctx context.Context) *sendState {
	st, _ := ctx.Value(sendStateKey{}).(*sendState)
	return st
}

// sendTrackingTransport sets sendState.sent when a request has been written to
// the connection in full, via httptrace's WroteRequest hook.
//
// "In full" is the point. WroteRequest also fires when the write fails, with the
// error in WroteRequestInfo, and a failed write means the server never received
// a complete request: a Content-Length body that falls short is rejected, and a
// chunked one without its terminating chunk is malformed. A package upload cut
// at 90% is therefore safe to start again, and treating it as sent would have
// refused to retry the very failure retrying exists for.
//
// It sits directly above the *http.Transport, below throttle, user-agent and
// header wrappers, so the trace is attached to the request that is actually
// written. A request carrying no sendState passes through untouched.
type sendTrackingTransport struct {
	base http.RoundTripper
}

// RoundTrip clears the per-attempt flag, attaches the trace, and delegates. The
// request is shallow-copied by WithContext, which RoundTripper's contract
// permits; the original is not mutated.
func (t *sendTrackingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	st := sendStateFrom(req.Context())
	if st == nil {
		return t.base.RoundTrip(req)
	}
	st.sent.Store(false)
	trace := &httptrace.ClientTrace{
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err == nil {
				st.sent.Store(true)
			}
		},
	}
	return t.base.RoundTrip(req.WithContext(httptrace.WithClientTrace(req.Context(), trace)))
}

// retryableTransportError reports whether a request that failed with err and
// produced no response is safe to send again.
//
// Before the request was fully written, always: nothing reached the server, so
// the method does not matter. After it, only an idempotent method, and never a
// timeout. The timeout carve-out is not about idempotency. A timeout after the
// send is the caller's own decision to stop waiting, or a gateway's, and
// retrying it multiplies that bound by the attempt count — one smart-group PUT
// that the gateway cuts off at 180s becomes five of them, each making the
// server recompute membership again.
//
// A nil st means the request was not tracked. That answers true, which is the
// behaviour before send tracking existed, rather than guessing the other way and
// silently disabling retries for a path the tracker does not cover.
func retryableTransportError(st *sendState, err error) bool {
	if st == nil || !st.sent.Load() {
		return true
	}
	if isTimeout(err) {
		return false
	}
	return isIdempotentMethod(st.method)
}

// isTimeout reports whether err, anywhere in its chain, is a timeout.
func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}
