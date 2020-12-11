package promise

import (
	"strings"
	"sync"
)

type promiseResult struct {
	Index int
	Data interface{}
	Err error
}

type Promise struct {
	lock *sync.Mutex
	result *promiseResult
	awaiters []chan bool
}

type ResolveFunc func(data interface{})
type RejectFunc func(err error)
type ThenFunc func(data interface{}) (interface{}, error)
type CatchFunc func(err error) (interface{}, error)
type AwaitFunc func() (interface{}, error)
type PromiseFunc func(resolve ResolveFunc, reject RejectFunc)

func NewPromise(fnc PromiseFunc) *Promise {
	p := &Promise{
		lock: &sync.Mutex{},
		result: nil,
		awaiters: make([]chan bool, 0),
	}
	resolve := func(data interface{}) {
		p.lock.Lock()
		p.result = &promiseResult{0, data, nil}
		awaiters := p.awaiters[:]
		p.awaiters = nil
		p.lock.Unlock()
		for _, ch := range awaiters {
			ch <- true
			close(ch)
		}
	}
	reject := func(err error) {
		p.lock.Lock()
		p.result = &promiseResult{0, nil, err}
		awaiters := p.awaiters[:]
		p.awaiters = nil
		p.lock.Unlock()
		for _, ch := range awaiters {
			ch <- false
			close(ch)
		}
	}
	go fnc(resolve, reject)
	return p
}

func NewAwaiter(fnc AwaitFunc) *Promise {
	pfunc := func(resolve ResolveFunc, reject RejectFunc) {
		data, err := fnc()
		if err != nil {
			reject(err)
		} else {
			resolve(data)
		}
	}
	return NewPromise(pfunc)
}

func (p *Promise) Await() (interface{}, error) {
	p.lock.Lock()
	if p.result != nil {
		p.lock.Unlock()
		return p.result.Data, p.result.Err
	}
	ch := make(chan bool, 1)
	p.awaiters = append(p.awaiters, ch)
	p.lock.Unlock()
	<-ch
	return p.result.Data, p.result.Err
}

func (p *Promise) Then(fnc func(data interface{}) (interface{}, error)) *Promise {
	thenFnc := func() (interface{}, error) {
		data, err := p.Await()
		if err == nil {
			return fnc(data)
		} else {
			return data, err
		}
	}
	return NewAwaiter(thenFnc)
}

func (p *Promise) Catch(fnc func(err error) (interface{}, error)) *Promise {
	catchFnc := func() (interface{}, error) {
		data, err := p.Await()
		if err == nil {
			return fnc(err)
		} else {
			return data, err
		}
	}
	return NewAwaiter(catchFnc)
}

type ErrList []error

func (el ErrList) Error() string {
	strs := make([]string, len(el))
	for i, err := range el {
		if err == nil {
			strs[i] = "<nil>"
		} else {
			strs[i] = err.Error()
		}
	}
	return strings.Join(strs, "; ")
}

func All(ps ...*Promise) *Promise {
	afunc := func() (interface{}, error) {
		n := len(ps)
		datums := make([]interface{}, n)
		errs := make([]error, n)
		hasErr := false
		ch := make(chan *promiseResult)
		for i, p := range ps {
			go func(index int, prom *Promise) {
				data, err := prom.Await()
				ch <- &promiseResult{index, data, err}
			}(i, p)
		}
		for n > 0 {
			res := <-ch
			datums[res.Index] = res.Data
			errs[res.Index] = res.Err
			if res.Err != nil {
				hasErr = true
			}
			n -= 1
		}
		if hasErr {
			return datums, ErrList(errs)
		}
		return datums, nil
	}
	return NewAwaiter(afunc)
}
