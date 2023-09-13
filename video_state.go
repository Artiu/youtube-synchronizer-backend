package main

import "time"

type VideoState struct {
	Path           string  `json:"path"`
	Time           float64 `json:"time"`
	Rate           float32 `json:"rate"`
	IsPaused       bool    `json:"isPaused"`
	lastTimeUpdate time.Time
}

func (v *VideoState) updatePredictedTime() {
	secondsDiff := float64(time.Now().UnixMilli()-v.lastTimeUpdate.UnixMilli()) / 1000
	v.Time += secondsDiff * float64(v.Rate)
}

func (v VideoState) GetPredicted() VideoState {
	if v.IsPaused {
		return v
	}
	v.updatePredictedTime()
	return v
}

func (v *VideoState) UpdatePath(newPath string) {
	if v.Path != newPath {
		v.Time = 0
		v.lastTimeUpdate = time.Now()
	}
	v.Path = newPath
}

func (v *VideoState) UpdateRate(newRate float32) {
	v.Rate = newRate
}

func (v *VideoState) UpdateIsPaused(isPaused bool) {
	if isPaused && !v.IsPaused {
		v.updatePredictedTime()
	}
	v.IsPaused = isPaused
	v.lastTimeUpdate = time.Now()
}

func (v *VideoState) UpdateTime(newTime float64) {
	v.Time = newTime
	v.lastTimeUpdate = time.Now()
}
