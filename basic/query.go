package main

import (
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/fiatjaf/go-nostr"
	"github.com/rs/zerolog/log"
)

func (b *BasicRelay) QueryEvents(filter *nostr.Filter) (events []nostr.Event, err error) {
	var conditions []string
	var params []interface{}

	if filter == nil {
		err = errors.New("filter cannot be null")
		return
	}

	if filter.IDs != nil {
		if len(filter.IDs) > 500 {
			// too many ids, fail everything
			return
		}

		inids := make([]string, 0, len(filter.IDs))
		for _, id := range filter.IDs {
			// to prevent sql attack here we will check if
			// these ids are valid 32byte hex
			parsed, err := hex.DecodeString(id)
			if err != nil || len(parsed) != 32 {
				continue
			}
			inids = append(inids, fmt.Sprintf("'%x'", parsed))
		}
		if len(inids) == 0 {
			// ids being [] mean you won't get anything
			return
		}
		conditions = append(conditions, `id IN (`+strings.Join(inids, ",")+`)`)
	}

	if filter.Authors != nil {
		if len(filter.Authors) > 500 {
			// too many authors, fail everything
			return
		}

		inkeys := make([]string, 0, len(filter.Authors))
		for _, key := range filter.Authors {
			// to prevent sql attack here we will check if
			// these keys are valid 32byte hex
			parsed, err := hex.DecodeString(key)
			if err != nil || len(parsed) != 32 {
				continue
			}
			inkeys = append(inkeys, fmt.Sprintf("'%x'", parsed))
		}
		if len(inkeys) == 0 {
			// authors being [] mean you won't get anything
			return
		}
		conditions = append(conditions, `pubkey IN (`+strings.Join(inkeys, ",")+`)`)
	}

	if filter.Kinds != nil {
		if len(filter.Kinds) > 10 {
			// too many kinds, fail everything
			return
		}

		if len(filter.Kinds) == 0 {
			// kinds being [] mean you won't get anything
			return
		}
		// no sql injection issues since these are ints
		inkinds := make([]string, len(filter.Kinds))
		for i, kind := range filter.Kinds {
			inkinds[i] = strconv.Itoa(kind)
		}
		conditions = append(conditions, `kind IN (`+strings.Join(inkinds, ",")+`)`)
	}

	tagQuery := make([]string, 0, 1)
	for _, values := range filter.Tags {
		if len(values) == 0 {
			// any tag set to [] is wrong
			return
		}

		// add these tags to the query
		tagQuery = append(tagQuery, values...)

		if len(tagQuery) > 10 {
			// too many tags, fail everything
			return
		}
	}

	if len(tagQuery) > 0 {
		arrayBuild := make([]string, len(tagQuery))
		for i, tagValue := range tagQuery {
			arrayBuild[i] = "?"
			params = append(params, tagValue)
		}

		// we use a very bad implementation in which we only check the tag values and
		// ignore the tag names
		conditions = append(conditions,
			"tagvalues && ARRAY["+strings.Join(arrayBuild, ",")+"]")
	}

	aLongTimeAgo := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
	if filter.Since.Before(aLongTimeAgo) {
		conditions = append(conditions, "created_at > ?")
		params = append(params, filter.Since.Unix())
	}
	if filter.Until.Before(aLongTimeAgo) {
		conditions = append(conditions, "created_at < ?")
		params = append(params, filter.Until.Unix())
	}

	if len(conditions) == 0 {
		// fallback
		conditions = append(conditions, "true")
	}

	query := b.DB.Rebind(`SELECT
      id, pubkey, created_at, kind, tags, content, sig
    FROM event WHERE ` +
		strings.Join(conditions, " AND ") +
		" ORDER BY created_at LIMIT 100")

	err = b.DB.Select(&events, query, params...)
	if err != nil && err != sql.ErrNoRows {
		log.Warn().Err(err).Interface("filter", filter).Str("query", query).
			Msg("failed to fetch events")
		err = fmt.Errorf("failed to fetch events: %w", err)
	}

	return
}
