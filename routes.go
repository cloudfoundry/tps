package tps

import "github.com/tedsuo/rata"

const (
	LRPStatus    = "LRPStatus"
	LRPStats     = "LRPStats"
	BulkLRPStats = "BulkLRPStats"
)

var Routes = rata.Routes{
	{Path: "/v1/actual_lrps/bulk_stats", Method: "GET", Name: BulkLRPStats},
	{Path: "/v1/actual_lrps/:guid", Method: "GET", Name: LRPStatus},
	{Path: "/v1/actual_lrps/:guid/stats", Method: "GET", Name: LRPStats},
}
