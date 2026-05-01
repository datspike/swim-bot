package bot

import "sync"

// RingEntry содержит информацию об одном сообщении в скользящем окне.
type RingEntry struct {
	IsSpam bool  // true = спам-сообщение
	UserID int64 // ID спамера (0 если не спам)
}

// RingBuffer — циклический буфер для скользящего окна сообщений чата.
// Потокобезопасен через sync.Mutex.
type RingBuffer struct {
	mu    sync.Mutex
	items []RingEntry
	pos   int // текущая позиция записи
	size  int // M — размер буфера
}

// NewRingBuffer создаёт новый буфер размером size.
func NewRingBuffer(size int) *RingBuffer {
	if size <= 0 {
		size = 20
	}
	return &RingBuffer{
		items: make([]RingEntry, size),
		size:  size,
	}
}

// Push добавляет запись в буфер. O(1).
func (rb *RingBuffer) Push(isSpam bool, userID int64) {
	rb.mu.Lock()
	rb.items[rb.pos] = RingEntry{IsSpam: isSpam, UserID: userID}
	rb.pos = (rb.pos + 1) % rb.size
	rb.mu.Unlock()
}

// SpamCountByOthers возвращает число спам-записей от других пользователей. O(M).
// Исключает записи с excludeUserID из подсчёта.
func (rb *RingBuffer) SpamCountByOthers(excludeUserID int64) int {
	rb.mu.Lock()
	count := 0
	for _, entry := range rb.items {
		if entry.IsSpam && entry.UserID != excludeUserID {
			count++
		}
	}
	rb.mu.Unlock()
	return count
}

// ChatRingBuffers управляет ring buffer'ами для всех чатов.
// Потокобезопасен через sync.Map, ленивая инициализация.
type ChatRingBuffers struct {
	buffers sync.Map // map[int64]*RingBuffer
}

// GetOrCreate возвращает ring buffer для чата, создавая при необходимости.
func (c *ChatRingBuffers) GetOrCreate(chatID int64, size int) *RingBuffer {
	if val, ok := c.buffers.Load(chatID); ok {
		if buf, ok := val.(*RingBuffer); ok {
			return buf
		}
	}
	buf := NewRingBuffer(size)
	actual, _ := c.buffers.LoadOrStore(chatID, buf)
	if stored, ok := actual.(*RingBuffer); ok {
		return stored
	}
	c.buffers.Store(chatID, buf)
	return buf
}
