// Copyright 2015 The Tango Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ssdbstore

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"reflect"
	"time"
	"unsafe"

	"github.com/lunny/log"
	"github.com/lunny/tango"
	"github.com/seefan/gossdb"
	"github.com/tango-contrib/session"
)

var _ session.Store = &SSDBStore{}

type Options struct {
	Host     string
	Port     int
	Password string
	DbIndex  int
	MaxAge   time.Duration
}

// SSDBStore represents a redis session store implementation.
type SSDBStore struct {
	Options
	Logger tango.Logger
	pool   *gossdb.Connectors
}

func (r *SSDBStore) maxSeconds() int64 {
	return int64(r.MaxAge / time.Second)
}

func preOptions(opts []Options) Options {
	var opt Options
	if len(opts) > 0 {
		opt = opts[0]
	}
	if opt.Host == "" {
		opt.Host = "127.0.0.1"
	}
	if opt.Port == 0 {
		opt.Port = 6380
	}
	if opt.MaxAge == 0 {
		opt.MaxAge = session.DefaultMaxAge
	}
	return opt
}

// NewSSDBStore creates and returns a redis session store.
func New(opts ...Options) (*SSDBStore, error) {
	opt := preOptions(opts)
	pool, err := gossdb.NewPool(&gossdb.Config{
		Host:             opt.Host,
		Port:             opt.Port,
		MinPoolSize:      5,
		MaxPoolSize:      50,
		AcquireIncrement: 5,
	})

	if err != nil {
		return nil, err
	}

	return &SSDBStore{
		Options: opt,
		pool:    pool,
		Logger:  log.Std,
	}, nil
}

func (c *SSDBStore) serialize(value interface{}) ([]byte, error) {
	err := c.registerGobConcreteType(value)
	if err != nil {
		return nil, err
	}

	if reflect.TypeOf(value).Kind() == reflect.Struct {
		return nil, fmt.Errorf("serialize func only take pointer of a struct")
	}

	var b bytes.Buffer
	encoder := gob.NewEncoder(&b)

	err = encoder.Encode(&value)
	if err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (c *SSDBStore) deserialize(byt []byte) (ptr interface{}, err error) {
	b := bytes.NewBuffer(byt)
	decoder := gob.NewDecoder(b)

	var p interface{}
	err = decoder.Decode(&p)
	if err != nil {
		return
	}

	v := reflect.ValueOf(p)
	if v.Kind() == reflect.Struct {
		var pp interface{} = &p
		datas := reflect.ValueOf(pp).Elem().InterfaceData()

		sp := reflect.NewAt(v.Type(),
			unsafe.Pointer(datas[1])).Interface()
		ptr = sp
	} else {
		ptr = p
	}
	return
}

func (c *SSDBStore) registerGobConcreteType(value interface{}) error {
	t := reflect.TypeOf(value)

	switch t.Kind() {
	case reflect.Ptr:
		v := reflect.ValueOf(value)
		i := v.Elem().Interface()
		gob.Register(i)
	case reflect.Struct, reflect.Map, reflect.Slice:
		gob.Register(value)
	case reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Bool, reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		// do nothing since already registered known type
	default:
		return fmt.Errorf("unhandled type: %v", t)
	}
	return nil
}

// Set sets value to given key in session.
func (s *SSDBStore) Set(id session.Id, key string, val interface{}) error {
	bs, err := s.serialize(val)
	if err != nil {
		return err
	}

	c, err := s.pool.NewClient()
	if err != nil {
		return err
	}
	defer c.Close()

	err = c.Hset(string(id), key, bs)
	if err == nil {
		_, err = c.Expire(string(id), s.maxSeconds())
	}

	return err
}

// Get gets value by given key in session.
func (s *SSDBStore) Get(id session.Id, key string) interface{} {
	c, err := s.pool.NewClient()
	if err != nil {
		s.Logger.Errorf("ssdb HGET %s failed: %s", string(id)+":"+key, err)
		return nil
	}
	defer c.Close()

	v, err := c.Hget(string(id), key)
	if err != nil {
		s.Logger.Errorf("ssdb HGET %s failed: %s", string(id)+":"+key, err)
		return nil
	}
	if v.IsEmpty() {
		return nil
	}

	_, err = c.Expire(string(id), s.maxSeconds())
	if err != nil {
		s.Logger.Errorf("ssdb HGET %s failed: %s", string(id)+":"+key, err)
		return nil
	}

	value, err := s.deserialize(v.Bytes())
	if err != nil {
		s.Logger.Errorf("ssdb HGET %s failed: %s %s", string(id)+":"+key, string(v), err)
		return nil
	}
	return value
}

// Delete delete a key from session.
func (s *SSDBStore) Del(id session.Id, key string) bool {
	c, err := s.pool.NewClient()
	if err != nil {
		s.Logger.Errorf("ssdb HGET failed: %s", err)
		return false
	}
	defer c.Close()

	err = c.Hdel(string(id), key)
	return err == nil
}

func (s *SSDBStore) Clear(id session.Id) bool {
	c, err := s.pool.NewClient()
	if err != nil {
		s.Logger.Errorf("ssdb HGET failed: %s", err)
		return false
	}
	defer c.Close()

	err = c.Del(string(id))
	return err == nil
}

func (s *SSDBStore) Add(id session.Id) bool {
	return true
}

func (s *SSDBStore) Exist(id session.Id) bool {
	c, err := s.pool.NewClient()
	if err != nil {
		s.Logger.Errorf("ssdb HGET failed: %s", err)
		return false
	}
	defer c.Close()
	has, err := c.Exists(string(id))
	return err == nil && has
}

func (s *SSDBStore) SetMaxAge(maxAge time.Duration) {
	s.MaxAge = maxAge
}

func (s *SSDBStore) SetIdMaxAge(id session.Id, maxAge time.Duration) {
	if s.Exist(id) {
		c, err := s.pool.NewClient()
		if err != nil {
			s.Logger.Errorf("ssdb HGET failed: %s", err)
			return
		}
		defer c.Close()

		_, err = c.Expire(string(id), int64(maxAge/time.Second))
		if err != nil {
			s.Logger.Errorf("ssdb HGET failed: %s", err)
			return
		}
	}
}

func (s *SSDBStore) Ping() error {
	c, err := s.pool.NewClient()
	if err != nil {
		return err
	}
	defer c.Close()

	if !c.Ping() {
		return errors.New("ping failed")
	}
	return nil
}

func (s *SSDBStore) Run() error {
	return s.Ping()
}
