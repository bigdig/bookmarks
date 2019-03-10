package queue

import (
	"github.com/nrocco/bookmarks/storage"
	"github.com/rs/zerolog/log"
)

// New returns a buffered channel that we can send work requests on.
func New(store *storage.Store, nworkers int) *Queue {
	log.Info().Int("workers", nworkers).Msg("Setting up the queue")

	queue := Queue{
		work:    make(chan workRequest, 100),
		workers: make(chan chan workRequest, nworkers),
	}

	for i := 0; i < nworkers; i++ {
		worker := worker{
			ID:      i,
			store:   store,
			work:    make(chan workRequest),
			workers: queue.workers,
		}
		log.Info().Int("worker", i).Msg("Starting worker")
		worker.Start()
	}

	queue.start()

	log.Info().Msg("Queue started")

	return &queue
}

// Queue is an object which accepts new work and manages work and workers
type Queue struct {
	work    chan workRequest
	workers chan chan workRequest
}

func (q *Queue) start() {
	go func() {
		for {
			select {
			case work := <-q.work:
				log.Info().Int64("work_id", work.ID).Str("work_type", work.Type).Msg("Got new work from the queue")
				go func() {
					worker := <-q.workers
					log.Info().Int64("work_id", work.ID).Str("work_type", work.Type).Msg("Moving work to a worker queue")
					worker <- work
				}()
			}
		}
	}()
}

// Schedule allows you to add new work to the Queue
func (q *Queue) Schedule(workType string, ID int64) {
	log.Info().Int64("work_id", ID).Str("work_type", workType).Msg("Scheduling work")

	q.work <- workRequest{Type: workType, ID: ID}
}

type worker struct {
	ID      int
	store   *storage.Store
	work    chan workRequest
	workers chan chan workRequest
}

func (w *worker) Start() {
	go func() {
		for {
			// Add ourselves into the worker queue.
			w.workers <- w.work

			select {
			case work := <-w.work:
				logger := log.With().Int("worker_id", w.ID).Int64("work_id", work.ID).Str("work_type", work.Type).Logger()

				if work.Type == "Bookmark.Fetch" {
					bookmark := storage.Bookmark{ID: work.ID}
					if err := w.store.GetBookmark(&bookmark); err != nil {
						logger.Warn().Err(err).Msg("Error loading bookmark")
						return
					}

					logger := logger.With().Str("bookmark_url", bookmark.URL).Logger()

					if err := bookmark.Fetch(); err != nil {
						logger.Warn().Err(err).Msg("Error fetching content")
						return
					}

					if err := w.store.UpdateBookmark(&bookmark); err != nil {
						logger.Warn().Err(err).Msg("Error saving content")
						return
					}

					logger.Info().Msg("Content for bookmark fetched")
				} else if work.Type == "Feed.Fetch" {
					feed := storage.Feed{ID: work.ID}
					if err := w.store.GetFeed(&feed); err != nil {
						logger.Warn().Err(err).Msg("Error loading feed")
						return
					}

					logger := logger.With().Str("feed_url", feed.URL).Logger()

					if err := w.store.RefreshFeed(&feed); err != nil {
						logger.Warn().Err(err).Msg("Error refreshing feed")
						return
					}

					logger.Info().Msg("Feed refreshed")
				} else {
					logger.Warn().Msg("Unknown work received")
				}

				logger.Info().Msg("Work is done")
			}
		}
	}()
}

type workRequest struct {
	Type string
	ID   int64
}
