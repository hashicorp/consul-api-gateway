package object

import (
	"context"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	statusUpdateTimeout     = 10 * time.Second
	statusFlushInterval     = 15 * time.Second
	maxStatusUpdateAttempts = 5
)

type StatusWorker struct {
	writer client.StatusWriter
	timer  *time.Timer

	queue   []*statusWorkerElem
	queueMu sync.Mutex

	logger hclog.Logger
}

func NewStatusWorker(ctx context.Context, writer client.StatusWriter, logger hclog.Logger) *StatusWorker {
	w := &StatusWorker{
		writer: writer,
		timer:  time.NewTimer(statusFlushInterval),
		logger: logger,
	}
	go w.writeLoop(ctx)
	return w
}

type statusWorkerElem struct {
	obj *Object
}

func (el *statusWorkerElem) writeStatus(writer client.StatusWriter) error {
	var err error
	if !el.obj.Status.IsDirty() {
		return nil
	}
	for i := 0; i < maxStatusUpdateAttempts; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), statusUpdateTimeout)
		err = writer.Update(ctx, el.obj.KubeObj)
		cancel()
		if err == nil {
			return nil
		}
	}
	return err
}

func (w *StatusWorker) Push(obj *Object) {
	w.queueMu.Lock()
	w.push(&statusWorkerElem{obj: obj})
	w.queueMu.Unlock()
}

func (w *StatusWorker) FlushAsync() {
	if !w.timer.Stop() {
		<-w.timer.C
	}
	w.timer.Reset(statusFlushInterval)
}

// queueMe lock must be held
func (w *StatusWorker) push(el *statusWorkerElem) {
	w.queue = append(w.queue, el)
}

// queueMe lock must be held
func (w *StatusWorker) pop() *statusWorkerElem {
	obj := w.queue[0]
	w.queue = append(w.queue[:0], w.queue[1:]...)
	return obj
}

func (w *StatusWorker) flushQueue() {
	w.queueMu.Lock()
	for i := 0; i < len(w.queue); i++ {
		el := w.pop()
		if err := el.writeStatus(w.writer); err != nil {
			w.logger.Error("failed to update object status after retrying", "error", err)
			continue
		}
		el.obj.Status.SetDirty(false)
	}
	w.queueMu.Unlock()
}

func (w *StatusWorker) writeLoop(ctx context.Context) {
	for {
		select {
		case <-w.timer.C:
			select {
			case <-ctx.Done():
				return
			default:
				w.flushQueue()
				w.timer.Reset(statusFlushInterval)
			}
		case <-ctx.Done():
			return
		}
	}
}
