package wiki

import (
	"github.com/RediSearch/ftsb/cmd/ftsb_generate_queries/utils"
	"github.com/RediSearch/ftsb/query"
)

// TwoWordIntersectionQuery contains info for filling simple 2 word queries with the barack obama words
type Simple2WordBarackObama struct {
	core utils.EnWikiAbstractGenerator
}

// NewTwoWordIntersectionQuery produces a new function that produces a new TwoWordIntersectionQuery
func NewSimple2WordBarackObama() utils.QueryFillerMaker {
	return func(core utils.EnWikiAbstractGenerator) utils.QueryFiller {
		return &Simple2WordBarackObama{
			core: core,
		}
	}
}

// Fill fills in the query.Query with query details
func (d *Simple2WordBarackObama) Fill(q query.Query) query.Query {
	fc, ok := d.core.(Simple2WordBarackObamaFiller)
	if !ok {
		panicUnimplementedQuery(d.core)
	}
	fc.Simple2WordBarackObama(q)
	return q
}
