package cron

import (
	"errors"
	"fmt"
	"log"
	"reflect"
	"time"
)

var (
	ErrTimeFormat = errors.New("time format error")
	ErrParamsNotAdapted = errors.New("the number of params is not adapted")
	ErrNotAFunction = errors.New("only functions can be schedule into the job queue")
	ErrPeriodNotSpecified = errors.New("unspecified job period")
	ErrParameterCannotBeNil = errors.New("nil parameters cannot be used with reflection")
)

type Job struct {
	interval uint64
	jobFunc string
	unit timeUnit
	atTime time.Duration
	err error
	loc *time.Location
	lastRun time.Time
	nextRun time.Time
	startDay time.Weekday
	funcs map[string]interface{}
	fparams map[string][]interface{}
	lock bool
	tags []string
}

func (j *Job) shouldRun() bool {
	return time.Now().Unix() >= j.nextRun.Unix()
}

func (j *Job) run() ([]reflect.Value, error) {
	if j.lock {
		if locker == nil {
			return nil, fmt.Errorf("trying to lock %s with nil locker", j.jobFunc)
		}
		key := getFunctionKey(j.jobFunc)

		locker.Lock(key)
		defer locker.Unlock(key)
	}
	result, err := callJobFuncWithParams(j.funcs[j.jobFunc], j.fparams[j.jobFunc])
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (j *Job) Err() error {
	return j.err
}

func (j *Job) Do(jobFun interface{}, params ...interface{}) error {
	if j.err != nil {
		return j.err
	}

	typ := reflect.TypeOf(jobFun)
	if typ.Kind() != reflect.Func {
		return ErrNotAFunction
	}
	fName := getFunctionName(jobFun)
	j.funcs[fName] = jobFun
	j.fparams[fName] = params
	j.jobFunc = fName

	now := time.Now().In(j.loc)
	if !j.nextRun.After(now) {
		_ = j.scheduleNextRun()
	}
	return nil
}

func (j *Job) DoSafely(jobFunc interface{}, params ...interface{}) error {
	recoveryWrapperFunc := func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Internal panic occured: %v", r)
			}
		}()
	}

	_, _ = callJobFuncWithParams(jobFunc, params)
	return j.Do(recoveryWrapperFunc)
}

func (j *Job) periodDuration() (time.Duration,error) {
	interval := time.Duration(j.interval)	
	var periodDuration time.Duration

	switch j.unit {
	case seconds:
		periodDuration = interval * time.Second
	case minutes:
		periodDuration = interval * time.Minute
	case hours:
		periodDuration = interval * time.Hour
	case days:
		periodDuration = interval * time.Hour * 24
	case weeks:
		periodDuration = interval * time.Hour * 24 * 7
	default:
		return 0, ErrPeriodNotSpecified
	}
	return periodDuration, nil
}

func (j *Job) roundToMidnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, j.loc)
}

func (j *Job) scheduleNextRun() error {
	now := time.Now()
	if j.lastRun == time.Unix(0, 0) {
		j.lastRun = now
	}

	periodDuration, err := j.periodDuration()
	if err != nil {
		return err
	}

	switch j.unit {
	case seconds, minutes, hours:
		j.nextRun = j.lastRun.Add(periodDuration)
	case days:
		j.nextRun = j.roundToMidnight(j.lastRun)
		j.nextRun = j.nextRun.Add(j.atTime)
	case weeks:
		j.nextRun = j.roundToMidnight(j.lastRun)
		dayDiff := int(j.startDay)
		dayDiff -= int(j.nextRun.Weekday())
		if dayDiff != 0 {
			j.nextRun = j.nextRun.Add(time.Duration(dayDiff) * 24 * time.Hour)
		}
		j.nextRun = j.nextRun.Add(j.atTime)
	}

	for j.nextRun.Before(now) || j.nextRun.Before(j.lastRun) {
		j.nextRun = j.nextRun.Add(periodDuration)
	}
	return nil
}