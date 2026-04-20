package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/jellydator/ttlcache/v3"
	jsoniter "github.com/json-iterator/go"
	"github.com/ttab/elephantine"
	"github.com/ttab/ttninjs"
)

type TTNINJSErrorCause string

const (
	TTNINJSErrorCauseNotFound    TTNINJSErrorCause = "not_found"
	TTNINJSErrorCauseInvalidURI  TTNINJSErrorCause = "invalid_uri"
	TTNINJSErrorCauseInvalidDoc  TTNINJSErrorCause = "invalid_doc"
	TTNINJSErrorCauseInvalidBody TTNINJSErrorCause = "invalid_body"
)

type TTNINJSPermanentError struct {
	Cause TTNINJSErrorCause
}

func (err TTNINJSPermanentError) Error() string {
	return "permanent error: " + string(err.Cause)
}

// MediaOptions configures the Media client.
type MediaOptions struct {
	Logger *slog.Logger
	Client *http.Client
	Host   string
	// Cache enables TTNINJS document caching. If nil, no caching is performed.
	Cache *CacheOptions
}

type Media struct {
	logger *slog.Logger
	client *http.Client
	host   string
	cache  *mediaCache
}

func NewMedia(opts MediaOptions) *Media {
	m := &Media{
		logger: opts.Logger,
		client: opts.Client,
		host:   opts.Host,
	}

	if opts.Cache != nil {
		m.cache = newCache(*opts.Cache)
	}

	return m
}

func (m *Media) GetRenderedTTNINJS(
	ctx context.Context, docURI string, _ []byte,
) (ttninjs.Document, error) {
	if m.cache == nil {
		return m.fetch(ctx, docURI)
	}

	if item := m.cache.store.Get(docURI); item != nil {
		return item.Value(), nil
	}

	result, err, _ := m.cache.group.Do(docURI, func() (any, error) {
		doc, err := m.fetch(context.Background(), docURI)
		if err != nil {
			return nil, err
		}

		m.cache.store.Set(docURI, doc, ttlcache.DefaultTTL)

		return doc, nil
	})
	if err != nil {
		return ttninjs.Document{}, err //nolint:wrapcheck
	}

	return result.(ttninjs.Document), nil //nolint:forcetypeassert
}

func (m *Media) fetch(
	ctx context.Context, docURI string,
) (_ ttninjs.Document, outError error) {
	parsedURI, err := url.Parse(docURI)
	if err != nil {
		return ttninjs.Document{}, fmt.Errorf("invalid document URI: %w",
			errors.Join(err, TTNINJSPermanentError{
				Cause: TTNINJSErrorCauseInvalidURI,
			}))
	}

	// Switch to the correct host for the environment and add a JSON suffix
	// to get the JSON representation of the document.
	parsedURI.Host = m.host
	parsedURI.Scheme = "https"
	parsedURI.Path += ".json"

	req, err := http.NewRequest(http.MethodGet, parsedURI.String(), nil)
	if err != nil {
		return ttninjs.Document{}, fmt.Errorf(
			"create request: %w",
			errors.Join(err, TTNINJSPermanentError{
				Cause: TTNINJSErrorCauseInvalidURI,
			}))
	}

	res, err := m.client.Do(req.WithContext(ctx)) //nolint: bodyclose
	if err != nil {
		return ttninjs.Document{}, fmt.Errorf(
			"perform request: %w", err)
	}

	defer elephantine.Close("media ttninjs body", res.Body, &outError)

	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return ttninjs.Document{}, fmt.Errorf(
			"document could not be found: %w",
			TTNINJSPermanentError{
				Cause: TTNINJSErrorCauseNotFound,
			})
	default:
		return ttninjs.Document{}, fmt.Errorf(
			"media API responded with: %s", res.Status)
	}

	payload, err := io.ReadAll(res.Body)
	if err != nil {
		return ttninjs.Document{}, fmt.Errorf(
			"read media response: %w", err)
	}

	var doc ttninjs.Document

	err = jsoniter.Unmarshal(payload, &doc)
	if err != nil {
		return ttninjs.Document{}, fmt.Errorf(
			"unmarshal document: %w",
			errors.Join(err, TTNINJSPermanentError{
				Cause: TTNINJSErrorCauseInvalidDoc,
			}))
	}

	return doc, nil
}
