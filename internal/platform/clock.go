package platform

import "time"

type Clock interface {
	NowUnix() int64
	NowUnixMilli() int64
}

type systemClock struct{}

func (systemClock) NowUnix() int64 {
	return time.Now().Unix()
}

func (systemClock) NowUnixMilli() int64 {
	return time.Now().UnixMilli()
}

var SystemClock Clock = systemClock{}
