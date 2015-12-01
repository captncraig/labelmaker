package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/garyburd/redigo/redis"
)

var pool *redis.Pool

func newRedisPool(server string, database int) *redis.Pool {
	return &redis.Pool{
		MaxIdle:     10,
		MaxActive:   10,
		Wait:        true,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", server, redis.DialDatabase(database))
			if err != nil {
				return nil, err
			}
			if _, err := c.Do("CLIENT", "SETNAME", "Labelmaker"); err != nil {
				c.Close()
				return nil, err
			}
			return c, err
		},
	}
}

func getRepoInfo(owner, name string) (*repoInfo, error) {
	conn := pool.Get()
	defer conn.Close()
	b, err := redis.Bytes(conn.Do("HGET", "repos", fmt.Sprintf("%s:%s", owner, name)))
	if err != nil {
		if err == redis.ErrNil {
			return nil, nil
		}
		return nil, err
	}
	ri := &repoInfo{}
	if err = json.Unmarshal(b, ri); err != nil {
		return nil, err
	}
	return ri, nil
}

func getHookInfo(hook string) (*hookInfo, error) {
	conn := pool.Get()
	defer conn.Close()
	b, err := redis.Bytes(conn.Do("HGET", "hooks", hook))
	if err != nil {
		return nil, err
	}
	ri := &hookInfo{}
	if err = json.Unmarshal(b, ri); err != nil {
		return nil, err
	}
	return ri, nil
}

func registerHook(owner, name, hookPath, hookSecret string, id int, token string) error {
	conn := pool.Get()
	defer conn.Close()

	hi := &hookInfo{
		Owner:  owner,
		Name:   name,
		Secret: hookSecret,
	}
	d, err := json.Marshal(hi)
	if err != nil {
		return err
	}
	if _, err = conn.Do("HSET", "hooks", hookPath, d); err != nil {
		return err
	}

	ri := &repoInfo{
		Owner:       owner,
		Name:        name,
		HookID:      id,
		AccessToken: token,
		HookPath:    hookPath,
	}
	d, err = json.Marshal(ri)
	if err != nil {
		return err
	}
	if _, err = conn.Do("HSET", "repos", fmt.Sprintf("%s:%s", owner, name), d); err != nil {
		return err
	}
	return nil
}

type repoInfo struct {
	Owner, Name string
	HookID      int
	AccessToken string
	HookPath    string
}

type hookInfo struct {
	Owner, Name string
	Secret      string
}
