package tracker

import (
	"encoding/json"
	"fmt"
	"github.com/tellor-io/TellorMiner/config"
	"github.com/tellor-io/TellorMiner/util"
	"math"
	"sort"
	"time"
)

type ValueProcessor interface {
	Transform(*PrespecifiedRequest, [][]byte) (float64, error)
	Value(*PrespecifiedRequest) (float64, bool)
}

type ValueP struct {
	ValueProcessor
}

func (f *ValueP)UnmarshalJSON(data []byte) error {
	var str string
	err := json.Unmarshal(data, &str)
	if err != nil {
		return err
	}
	switch str {
	case "median": f.ValueProcessor = MedianProc{}
	case "value": f.ValueProcessor = DefaultProcessor{}
	case "dayAvg": f.ValueProcessor = TimeAverage{}
	case "": f.ValueProcessor = DefaultProcessor{}
	default: return fmt.Errorf("unrecognized transformation in PSR: %s", str)
	}
	return nil
}

//defaults for all PSRs
//override these functions to change behaviour
type DefaultProcessor struct {
}

func (p DefaultProcessor)Transform(r *PrespecifiedRequest, payloads [][]byte) (float64, error) {
	return mean(parsePayloads(r, payloads)), nil
}

func (p DefaultProcessor)Value(r *PrespecifiedRequest) (float64, bool) {
	ti := GetLatestRequestValue(r.RequestID)
	return ti.Val, true
}



type MedianProc struct {
	DefaultProcessor
}
func (m MedianProc)Transform(r *PrespecifiedRequest, payloads [][]byte) (float64, error) {
	return median(parsePayloads(r, payloads)), nil
}


//does a time average over 1 day
type TimeAverage struct {
	DefaultProcessor
}
func (t TimeAverage)Value(r *PrespecifiedRequest) (float64, bool) {
	cfg := config.GetConfig()

	timeInterval := 24 * time.Hour

	//get all the data we have saved locally for the past day
	now := time.Now()
	vals := GetRequestValuesForTime(r.RequestID, now, timeInterval)

	max := timeInterval/cfg.TrackerSleepCycle.Duration

	//require at least 60% of the values from the past day
	ratio := float64(len(vals))/float64(max)
	if ratio < 0.6 {
		//this isn't perfectly accurate, but it's a good cheap estimate. And is this REALLY worth the time to do right?
		estimate := time.Duration((0.6-ratio) * float64(timeInterval))
		psrLog.Info("Insufficient data for request ID %d, expected in %s", r.RequestID, estimate.String())
		return 0, false
	}

	floatVals := make([]float64, len(vals))
	for i := range vals {
		floatVals[i] = vals[i].Val
	}
	return mean(floatVals), true
}



//common shared functions among different value processors

func parsePayloads(r *PrespecifiedRequest, payloads [][]byte) []float64 {
	vals := make([]float64, 0, len(payloads))
	for i, pl := range payloads {
		if pl == nil {
			continue
		}
		v, err := util.ParsePayload(pl, r.Granularity, r.argGroups[i])
		if err != nil {
			continue
		}
		vals = append(vals, v)
	}
	if len(vals) == 0 {
		psrLog.Error("no sucessful api hits, no value stored for id %d", r.RequestID)
	}
	return vals
}

//an alternative weighting scheme.
//
// new values     1.00
// 6 hours old    0.50
// 24 hours old   0.05
func expTimeWeightedMean(vals []*TimedFloat) float64 {
	if len(vals) == 0 {
		return 0
	}

	now := time.Now()
	sum := 0.0
	for _,v := range vals {
		delta := now.Sub(v.Created).Seconds()
		sum += v.Val * 1/(math.Exp(delta/(86400/3)))
	}
	return sum/float64(len(vals))
}

func mean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	//compute the mean
	sum := 0.0
	for _,v := range vals {
		sum += v
	}
	avg:= sum / float64(len(vals))
	return avg
}

func median(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}

	sort.Slice(vals, func (i, j int) bool {
		return vals[i] < vals[j]
	})
	return vals[len(vals)/2]
}