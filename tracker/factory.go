package tracker

import "fmt"

//CreateTracker a tracker instance by its well-known name
func createTracker(name string) ([]Tracker, error) {
	switch name {
	case "test":
		{
			return []Tracker{&TestTracker{}}, nil
		}
	case "balance":
		{
			return []Tracker{&BalanceTracker{}}, nil
		}
	case "currentVariables":
		{
			return []Tracker{&CurrentVariablesTracker{}}, nil
		}
	case "disputeStatus":
		{
			return []Tracker{&DisputeTracker{}}, nil
		}
	case "gas":
		{
			return []Tracker{&GasTracker{}}, nil
		}
	case "top50":
		{
			return []Tracker{&Top50Tracker{}}, nil
		}
	case "tributeBalance":
		{
			return []Tracker{&TributeTracker{}}, nil
		}
	case "fetchData":
		{
			return []Tracker{&RequestDataTracker{}}, nil
		}
	case "psr":
		{
			return BuildPSRTrackers()
		}
	case "disputeChecker":
		return []Tracker{&disputeChecker{}}, nil
	default:
		return nil, fmt.Errorf("no tracker with the name %s", name)
	}
	return nil, nil
}
