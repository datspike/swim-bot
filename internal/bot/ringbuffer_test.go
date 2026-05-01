package bot

import (
	"sync"
	"testing"
)

func TestNewRingBuffer_DefaultSize(t *testing.T) {
	rb := NewRingBuffer(0)
	if rb.size != 20 {
		t.Errorf("ожидался размер 20 при size=0, получено %d", rb.size)
	}
}

func TestRingBuffer_Push_SpamCountByOthers(t *testing.T) {
	rb := NewRingBuffer(5)

	// пустой буфер — 0 спама
	if got := rb.SpamCountByOthers(100); got != 0 {
		t.Errorf("пустой буфер: ожидалось 0, получено %d", got)
	}

	// добавляем спам от пользователя 1
	rb.Push(true, 1)
	if got := rb.SpamCountByOthers(1); got != 0 {
		t.Errorf("собственный спам: ожидалось 0, получено %d", got)
	}
	if got := rb.SpamCountByOthers(2); got != 1 {
		t.Errorf("чужой спам: ожидалось 1, получено %d", got)
	}

	// добавляем не-спам
	rb.Push(false, 0)
	if got := rb.SpamCountByOthers(2); got != 1 {
		t.Errorf("после не-спама: ожидалось 1, получено %d", got)
	}

	// добавляем спам от пользователя 2
	rb.Push(true, 2)
	if got := rb.SpamCountByOthers(2); got != 1 {
		t.Errorf("исключение себя: ожидалось 1, получено %d", got)
	}
	if got := rb.SpamCountByOthers(3); got != 2 {
		t.Errorf("два чужих спама: ожидалось 2, получено %d", got)
	}
}

func TestRingBuffer_CircularOverflow(t *testing.T) {
	rb := NewRingBuffer(3)

	// заполняем спамом от пользователей 1, 2, 3
	rb.Push(true, 1)
	rb.Push(true, 2)
	rb.Push(true, 3)

	if got := rb.SpamCountByOthers(99); got != 3 {
		t.Errorf("полный буфер: ожидалось 3, получено %d", got)
	}

	// перезапись: не-спам затирает запись пользователя 1
	rb.Push(false, 0)

	if got := rb.SpamCountByOthers(99); got != 2 {
		t.Errorf("после перезаписи: ожидалось 2, получено %d", got)
	}

	// ещё одна перезапись: затирает запись пользователя 2
	rb.Push(false, 0)
	if got := rb.SpamCountByOthers(99); got != 1 {
		t.Errorf("вторая перезапись: ожидалось 1, получено %d", got)
	}
}

func TestRingBuffer_ConcurrentAccess(t *testing.T) {
	t.Log("проверка конкурентного доступа")
	rb := NewRingBuffer(100)
	var wg sync.WaitGroup

	// параллельная запись
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			rb.Push(true, id)
		}(int64(i))
	}

	// параллельное чтение
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			rb.SpamCountByOthers(id)
		}(int64(i))
	}

	wg.Wait()
	// если дошли без deadlock/panic — тест пройден
}

func TestChatRingBuffers_GetOrCreate(t *testing.T) {
	var buffers ChatRingBuffers

	// ленивая инициализация
	buf1 := buffers.GetOrCreate(100, 10)
	if buf1 == nil {
		t.Fatal("GetOrCreate вернул nil")
	}
	if buf1.size != 10 {
		t.Errorf("ожидался размер 10, получено %d", buf1.size)
	}

	// повторный вызов возвращает тот же буфер
	buf2 := buffers.GetOrCreate(100, 20)
	if buf1 != buf2 {
		t.Error("GetOrCreate вернул новый буфер для существующего чата")
	}
	// размер не меняется
	if buf2.size != 10 {
		t.Errorf("размер не должен меняться: ожидалось 10, получено %d", buf2.size)
	}

	// другой чат — другой буфер
	buf3 := buffers.GetOrCreate(200, 5)
	if buf3 == buf1 {
		t.Error("разные чаты получили один буфер")
	}
}
