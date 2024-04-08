package binance

import (
	"github.com/go-kratos/kratos/v2/log"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// WsHandler handle raw websocket message
type WsHandler func(message []byte)

// ErrHandler handles errors
type ErrHandler func(err error)

// WsConfig webservice configuration
type WsConfig struct {
	Endpoint string
}

func newWsConfig(endpoint string) *WsConfig {
	return &WsConfig{
		Endpoint: endpoint,
	}
}

var wsServe = func(cfg *WsConfig, handler WsHandler, errHandler ErrHandler) (doneC, stopC chan struct{}, err error) {
	Dialer := websocket.Dialer{
		Proxy:             http.ProxyFromEnvironment,
		HandshakeTimeout:  45 * time.Second,
		EnableCompression: false,
	}

	c, _, err := Dialer.Dial(cfg.Endpoint, nil)
	if err != nil {
		return nil, nil, err
	}
	c.SetReadLimit(655350)
	doneC = make(chan struct{})
	stopC = make(chan struct{})
	go func() {
		// This function will exit either on error from
		// websocket.Conn.ReadMessage or when the stopC channel is
		// closed by the client.
		defer close(doneC)
		if WebsocketKeepalive {
			keepAlive(c, WebsocketTimeout)
		}
		// Wait for the stopC channel to be closed.  We do that in a
		// separate goroutine because ReadMessage is a blocking
		// operation.
		silent := false
		go func() {
			select {
			case <-stopC:
				silent = true
			case <-doneC:
			}
			c.Close()
		}()
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Errorf("ReadMessage, err:%v", err)
				if !silent {
					errHandler(err)
				}
				return
			}
			handler(message)
		}
	}()
	return
}

func keepAlive(c *websocket.Conn, timeout time.Duration) {
	ticker := time.NewTicker(timeout)

	lastResponse := time.Now()
	c.SetPongHandler(func(msg string) error {
		log.Infof("keepAlive, SetPongHandler, msg:%v", msg)
		lastResponse = time.Now()
		return nil
	})

	c.SetPingHandler(func(msg string) error {
		log.Infof("keepAlive, SetPingHandler, msg:%v", msg)
		err := c.WriteControl(websocket.PongMessage, []byte(msg), time.Now().Add(time.Second))
		if err == websocket.ErrCloseSent {
			return nil
		} else if _, ok := err.(net.Error); ok {
			return nil
		}
		return err
	})

	go func() {
		defer ticker.Stop()
		for {
			deadline := time.Now().Add(10 * time.Second)
			log.Infof("keepAlive, WriteControl:%v", deadline)
			err := c.WriteControl(websocket.PingMessage, []byte{}, deadline)
			if err != nil {
				log.Errorf("keepAlive, WriteControl, err:%v", err)
				return
			}
			<-ticker.C
			if time.Since(lastResponse) > timeout {
				log.Infof("keepAlive, Close")
				c.Close()
				return
			}
		}
	}()
}
