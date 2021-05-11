package cron

import (
	"errors"
	"fmt"
	"log"
	"reflect"
	"time"
)

var (
	ErrTimeFormat           = errors.New("time format error")
	ErrParamsNotAdapted     = errors.New("the number of params is not adapted")
	ErrNotAFunction         = errors.New("only functions can be schedule into the job queue")
	ErrPeriodNotSpecified   = errors.New("unspecified job period")
	ErrParameterCannotBeNil = errors.New("nil parameters cannot be used with reflection")
)

type Job struct {
	interval uint64
	jobFunc  string
	unit     timeUnit
	atTime   time.Duration
	err      error
	loc      *time.Location
	lastRun  time.Time
	nextRun  time.Time
	startDay time.Weekday
	funcs    map[string]interface{}
	fparams  map[string][]interface{}
	lock     bool
	tags     []string
}

func NewJob(interval uint64) *Job {
	return &Job{
		interval: interval,
		loc:      time.Local,
		lastRun:  time.Unix(0, 0),
		nextRun:  time.Unix(0, 0),
		startDay: time.Sunday,
		funcs:    make(map[string]interface{}),
		fparams:  make(map[string][]interface{}),
		tags:     []string{},
	}
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

func (j *Job) At(t string) *Job {
	hour, min, sec, err := formatTime(t)
	if err != nil {
		j.err = ErrTimeFormat
		return j
	}
	j.atTime = time.Duration(hour)*time.Hour + time.Duration(min)*time.Minute + time.Duration(sec)*time.Second
	return j
}

func (j *Job) GetAt() string {
	return fmt.Sprintf("%d:%d", j.atTime/time.Hour, (j.atTime%time.Hour)/time.Minute)
}

func (j *Job) Loc(loc *time.Location) *Job {
	j.loc = loc
	return j
}

func (j *Job) Tag(t string, others ...string) {
	j.tags = append(j.tags, t)
	for _, tag := range others {
		j.tags = append(j.tags, tag)
	}
}

func (j *Job) Untag(t string) {
	var newTags []string
	for _, tag := range j.tags {
		if t != tag {
			newTags = append(newTags, tag)
		}
	}
	j.tags = newTags
}

func (j *Job) Tags() []string {
	return j.tags
}

func (j *Job) periodDuration() (time.Duration, error) {
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

func (j *Job) NextScheduledTime() time.Time {
	return j.nextRun
}

func (j *Job) mustInterval(i uint64) error {
	if j.interval != i {
		return fmt.Errorf("interval must be %d", i)
	}
	return nil
}

func (j *Job) From(t *time.Time) *Job {
	j.nextRun = *t
	return j
}

func (j *Job) setUnit(unit timeUnit) *Job {
	j.unit = unit
	return j
}

func (j *Job) Seconds() *Job {
	return j.setUnit(seconds)
}

func (j *Job) Minutes() *Job {
	return j.setUnit(minutes)
}

func (j *Job) Hours() *Job {
	return j.setUnit(hours)
}

func (j *Job) Days() *Job {
	return j.setUnit(days)
}

func (j *Job) Weeks() *Job {
	return j.setUnit(weeks)
}

func (j *Job) Second() *Job {
	_ = j.mustInterval(1)
	return j.Seconds()
}

func (j *Job) Minute() *Job {
	_ = j.mustInterval(1)
	return j.Minutes()
}

func (j *Job) Hour() *Job {
	_ = j.mustInterval(1)
	return j.Hours()
}

func (j *Job) Day() *Job {
	_ = j.mustInterval(1)
	return j.Days()
}

func (j *Job) Week() *Job {
	_ = j.mustInterval(1)
	return j.Weeks()
}

func (j *Job) Weekday(startDay time.Weekday) *Job {
	_ = j.mustInterval(1)
	j.startDay = startDay
	return j.Weeks()
}
func (j *Job) GetWeekday() time.Weekday {
	return j.startDay
}

// Monday set the start day with Monday
// - s.Every(1).Monday().Do(task)
func (j *Job) Monday() (job *Job) {
	return j.Weekday(time.Monday)
}

// Tuesday sets the job start day Tuesday
func (j *Job) Tuesday() *Job {
	return j.Weekday(time.Tuesday)
}

// Wednesday sets the job start day Wednesday
func (j *Job) Wednesday() *Job {
	return j.Weekday(time.Wednesday)
}

// Thursday sets the job start day Thursday
func (j *Job) Thursday() *Job {
	return j.Weekday(time.Thursday)
}

// Friday sets the job start day Friday
func (j *Job) Friday() *Job {
	return j.Weekday(time.Friday)
}

// Saturday sets the job start day Saturday
func (j *Job) Saturday() *Job {
	return j.Weekday(time.Saturday)
}

// Sunday sets the job start day Sunday
func (j *Job) Sunday() *Job {
	return j.Weekday(time.Sunday)
}

// Lock prevents job to run from multiple instances of gocron
func (j *Job) Lock() *Job {
	j.lock = true
	return j
}
