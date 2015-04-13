package tps

import "github.com/tedsuo/rata"

const (
	LRPStatus = "LRPStatus"
)

var Routes = rata.Routes{
	{Path: "/v1/actual_lrps/:guid", Method: "GET", Name: LRPStatus},
}
