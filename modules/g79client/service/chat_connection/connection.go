package chat_connection

import (
	"context"
	"crypto/aes"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"

	g79client "github.com/xingbaiawa/g79client"
	"github.com/xingbaiawa/g79client/utils"
)

// 表示一个与聊天服务器的活跃连接
type ChatConnection struct {
	service *ChatConnectionService
	entry   g79client.G79ChatServerEntry
	conn    net.Conn

	encrypt *utils.ChaChaEngine
	decrypt *utils.ChaChaEngine

	seqMu sync.Mutex
	seq   uint16

	sendMu sync.Mutex

	messages chan Message
	closed   chan struct{}

	closeOnce sync.Once
	wg        sync.WaitGroup

	errMu   sync.Mutex
	lastErr error
	online  atomic.Bool
}

func newChatConnection(service *ChatConnectionService, conn net.Conn, entry g79client.G79ChatServerEntry) *ChatConnection {
	return &ChatConnection{
		service:  service,
		entry:    entry,
		conn:     conn,
		messages: make(chan Message, 32),
		closed:   make(chan struct{}),
	}
}

func (c *ChatConnection) handshake(ctx context.Context) error {
	tokenBytes, err := decodeToken(c.service.client.UserToken)
	if err != nil {
		return fmt.Errorf("handshake: %v", err)
	}

	uid64, err := c.service.client.GetUserIDInt()
	if err != nil {
		return fmt.Errorf("handshake: %v", err)
	}
	if uid64 < 0 || uid64 > math.MaxUint32 {
		return fmt.Errorf("handshake: User ID is outside the uint32 range: %d", uid64)
	}
	uid := uint32(uid64)

	randBytes := make([]byte, 16)
	if _, err := rand.Read(randBytes); err != nil {
		return fmt.Errorf("handshake: %v", err)
	}

	block, err := aes.NewCipher(tokenBytes)
	if err != nil {
		return fmt.Errorf("handshake: %v", err)
	}

	aesRand := make([]byte, 16)
	block.Encrypt(aesRand, randBytes)

	head := make([]byte, 2)
	binary.LittleEndian.PutUint16(head, 1)

	body := make([]byte, 2+len(randBytes)+len(aesRand)+4)
	binary.LittleEndian.PutUint16(body[:2], handshakeRequestCmd)
	copy(body[2:18], randBytes)
	copy(body[18:34], aesRand)
	binary.LittleEndian.PutUint32(body[34:], uid)

	payload := append(head, body...)
	framed, err := wrapFrame(payload)
	if err != nil {
		return fmt.Errorf("handshake: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := c.conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("handshake: %v", err)
	}
	defer func() {
		_ = c.conn.SetDeadline(time.Time{})
	}()

	if err := c.sendRaw(framed); err != nil {
		return fmt.Errorf("handshake: %v", err)
	}

	for {
		frame, err := readFrame(c.conn)
		if err != nil {
			return fmt.Errorf("handshake: %v", err)
		}
		if len(frame) == 0 {
			continue
		}
		if len(frame) < 5 {
			return fmt.Errorf("handshake: Handshake response is too short: %d", len(frame))
		}
		ret := frame[4]
		if ret != 0 {
			return fmt.Errorf("handshake: Server returned code %d", ret)
		}
		break
	}

	encryptKey := append(append([]byte{}, tokenBytes...), randBytes...)
	decryptKey := append(append([]byte{}, randBytes...), tokenBytes...)

	encryptEngine, err := utils.NewNeteaseChaCha(encryptKey)
	if err != nil {
		return fmt.Errorf("handshake: %v", err)
	}
	decryptEngine, err := utils.NewNeteaseChaCha(decryptKey)
	if err != nil {
		return fmt.Errorf("handshake: %v", err)
	}

	c.encrypt = encryptEngine
	c.decrypt = decryptEngine

	c.seqMu.Lock()
	c.seq = 2
	c.seqMu.Unlock()

	c.online.Store(true)
	return nil
}

func (c *ChatConnection) startLoops() {
	c.wg.Add(2)
	go c.readLoop()
	go c.heartbeatLoop()
}

func (c *ChatConnection) readLoop() {
	defer c.wg.Done()
	defer close(c.messages)

	for {
		frame, err := readFrame(c.conn)
		if err != nil {
			c.setError(err)
			return
		}
		if len(frame) == 0 {
			continue
		}
		if len(frame) < 2 {
			c.setError(fmt.Errorf("readLoop: Invalid frame length %d", len(frame)))
			return
		}

		seq := binary.LittleEndian.Uint16(frame[:2])
		encrypted := frame[2:]
		plain, err := c.decrypt.Process(encrypted)
		if err != nil {
			c.setError(fmt.Errorf("readLoop: %v", err))
			return
		}
		if len(plain) < 2 {
			continue
		}
		cmd := binary.LittleEndian.Uint16(plain[:2])
		payload := append([]byte(nil), plain[2:]...)

		select {
		case c.messages <- Message{Sequence: seq, Command: cmd, Payload: payload}:
		case <-c.closed:
			return
		}
	}
}

func (c *ChatConnection) heartbeatLoop() {
	defer c.wg.Done()
	ticker := time.NewTicker(heartbeatIntervalSeconds * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := c.sendRaw(emptyFrame); err != nil {
				c.setError(fmt.Errorf("heartbeatLoop: %v", err))
				return
			}
		case <-c.closed:
			return
		}
	}
}

// 发送私聊消息，返回本次使用的序列号
func (c *ChatConnection) ChatTo(uid any, words string) (uint16, error) {
	if c == nil {
		return 0, fmt.Errorf("ChatTo: Connection is nil")
	}
	payload, err := json.Marshal([]any{uid, words})
	if err != nil {
		return 0, fmt.Errorf("ChatTo: %v", err)
	}
	return c.SendCommand(CommandChatPrivate, payload)
}

// 发送群聊消息，返回本次使用的序列号
func (c *ChatConnection) GroupChatTo(groupID any, words string) (uint16, error) {
	if c == nil {
		return 0, fmt.Errorf("GroupChatTo: Connection is nil")
	}
	payload, err := json.Marshal([]any{groupID, words})
	if err != nil {
		return 0, fmt.Errorf("GroupChatTo: %v", err)
	}
	return c.SendCommand(CommandChatGroup, payload)
}

func (c *ChatConnection) sendRaw(data []byte) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	return c.writeRaw(data)
}

func (c *ChatConnection) writeRaw(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	for len(data) > 0 {
		written, err := c.conn.Write(data)
		if err != nil {
			return fmt.Errorf("writeRaw: %v", err)
		}
		if written == 0 {
			return fmt.Errorf("writeRaw: Connection wrote zero bytes")
		}
		data = data[written:]
	}
	return nil
}

// 发送一条聊天命令，并返回使用的序列号
func (c *ChatConnection) SendCommand(cmd uint16, payload []byte) (uint16, error) {
	if c == nil || c.encrypt == nil {
		return 0, fmt.Errorf("SendCommand: Connection is not ready")
	}
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	plain := make([]byte, 2+len(payload))
	binary.LittleEndian.PutUint16(plain[:2], cmd)
	copy(plain[2:], payload)

	c.seqMu.Lock()
	seq := c.seq
	if seq == 0 {
		seq = 1
	}
	c.seq++
	c.seqMu.Unlock()

	encrypted, err := c.encrypt.Process(plain)
	if err != nil {
		return 0, fmt.Errorf("SendCommand: %v", err)
	}

	msg := make([]byte, 2+len(encrypted))
	binary.LittleEndian.PutUint16(msg[:2], seq)
	copy(msg[2:], encrypted)

	framed, err := wrapFrame(msg)
	if err != nil {
		return 0, fmt.Errorf("SendCommand: %v", err)
	}

	if err := c.writeRaw(framed); err != nil {
		return 0, fmt.Errorf("SendCommand: %v", err)
	}
	return seq, nil
}

// 返回连接是否已登录
func (c *ChatConnection) IsOnline() bool {
	return c.online.Load()
}

// 返回当前序列号
func (c *ChatConnection) SerialNumber() uint16 {
	c.seqMu.Lock()
	defer c.seqMu.Unlock()
	return c.seq
}

// 返回用于读取服务器推送消息的只读通道
func (c *ChatConnection) Messages() <-chan Message {
	return c.messages
}

// 关闭连接并释放资源
func (c *ChatConnection) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.closed)
		err = c.conn.Close()
		c.online.Store(false)
	})
	c.wg.Wait()
	return err
}

// 返回读写循环中出现的最新错误
func (c *ChatConnection) Err() error {
	c.errMu.Lock()
	defer c.errMu.Unlock()
	return c.lastErr
}

func (c *ChatConnection) setError(err error) {
	if err == io.EOF {
		// 正常断线也需要通知外部
	}
	c.errMu.Lock()
	if c.lastErr == nil {
		c.lastErr = err
	}
	c.errMu.Unlock()
	c.online.Store(false)
	c.closeOnce.Do(func() {
		close(c.closed)
		_ = c.conn.Close()
	})
}

func readFrame(r io.Reader) ([]byte, error) {
	lengthBuf := make([]byte, 2)
	if _, err := io.ReadFull(r, lengthBuf); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint16(lengthBuf)
	if size == 0 {
		return nil, nil
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}
