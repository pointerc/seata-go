/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package getty

import (
	"sync"
	"sync/atomic"
	"time"

	getty "github.com/apache/dubbo-getty"
)

const (
	maxCheckAliveRetry = 600
	checkAliveInternal = 100
)

var (
	sessionManager = newSessionManager()
)

type SessionManager struct {
	// serverAddress -> rpc_client.Session -> bool
	serverSessions sync.Map
	allSessions    sync.Map
	sessionSize    int32
}

func newSessionManager() *SessionManager {
	return &SessionManager{
		allSessions: sync.Map{},
		// serverAddress -> rpc_client.Session -> bool
		serverSessions: sync.Map{},
	}
}

func (g *SessionManager) selectSession() getty.Session {
	var session getty.Session
	g.allSessions.Range(func(key, value interface{}) bool {
		session = key.(getty.Session)
		if session.IsClosed() {
			g.releaseSession(session)
		} else {
			return false
		}
		return true
	})
	if session != nil {
		return session
	}
	if g.sessionSize == 0 {
		ticker := time.NewTicker(time.Duration(checkAliveInternal) * time.Millisecond)
		defer ticker.Stop()
		for i := 0; i < maxCheckAliveRetry; i++ {
			<-ticker.C
			g.allSessions.Range(func(key, value interface{}) bool {
				session = key.(getty.Session)
				if session.IsClosed() {
					g.releaseSession(session)
				} else {
					return false
				}
				return true
			})
			if session != nil {
				return session
			}
		}
	}
	return nil
}

func (g *SessionManager) releaseSession(session getty.Session) {
	g.allSessions.Delete(session)
	if !session.IsClosed() {
		m, _ := g.serverSessions.LoadOrStore(session.RemoteAddr(), &sync.Map{})
		sMap := m.(*sync.Map)
		sMap.Delete(session)
		session.Close()
	}
	atomic.AddInt32(&g.sessionSize, -1)
}

func (g *SessionManager) registerSession(session getty.Session) {
	g.allSessions.Store(session, true)
	m, _ := g.serverSessions.LoadOrStore(session.RemoteAddr(), &sync.Map{})
	sMap := m.(*sync.Map)
	sMap.Store(session, true)
	atomic.AddInt32(&g.sessionSize, 1)
}
