session-ssdb [![Build Status](https://drone.io/github.com/tango-contrib/session-ssdb/status.png)](https://drone.io/github.com/tango-contrib/session-ssdb/latest) [![](http://gocover.io/_badge/github.com/tango-contrib/session-redis)](http://gocover.io/github.com/tango-contrib/session-ssdb)
======

Session-ssdb is a store of [session](https://github.com/tango-contrib/session) middleware for [Tango](https://github.com/lunny/tango) stored session data via [ssdb](http://ssdb.io).

## Installation

    go get github.com/tango-contrib/session-ssdb

## Simple Example

```Go
package main

import (
    "github.com/lunny/tango"
    "github.com/tango-contrib/session"
    "github.com/tango-contrib/session-ssdb"
)

type SessionAction struct {
    session.Session
}

func (a *SessionAction) Get() string {
    a.Session.Set("test", "1")
    return a.Session.Get("test").(string)
}

func main() {
    o := tango.Classic()
    store, err := ssdbstore.New(redistore.Options{
            Host:    "127.0.0.1",
            DbIndex: 0,
            MaxAge:  30 * time.Minute,
        }
    o.Use(session.New(session.Options{
        Store: store),
        }))
    o.Get("/", new(SessionAction))
}
```

## Getting Help

- [API Reference](https://gowalker.org/github.com/tango-contrib/session-ssdb)

## License

This project is under BSD License. See the [LICENSE](LICENSE) file for the full license text.
