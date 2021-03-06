package scheduler

import (
	"sync"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jobs"
)

// triggerGlobalStorage interface is used to represent a persistent layer on
// which triggers are stored.
type triggerGlobalStorage interface {
	GetAll() ([]*TriggerInfos, error)
	Add(trigger Trigger) error
	Delete(trigger Trigger) error
}

// globalDBStorage implements the triggerGlobalStorage interface and uses a
// single database in CouchDB as the underlying storage for triggers.
type globalDBStorage struct{}

// newGlobalDBStorage returns a new instance of CouchStorage using the
// specified database.
func newGlobalDBStorage() triggerGlobalStorage {
	return &globalDBStorage{}
}

func (s *globalDBStorage) GetAll() ([]*TriggerInfos, error) {
	var infos []*TriggerInfos
	// TODO(pagination): use a sort of couchdb.WalkDocs function when available.
	req := &couchdb.AllDocsRequest{Limit: 1000}
	err := couchdb.GetAllDocs(couchdb.GlobalTriggersDB, consts.Triggers, req, &infos)
	if err != nil {
		if couchdb.IsNoDatabaseError(err) {
			return infos, nil
		}
		return nil, err
	}
	return infos, nil
}

func (s *globalDBStorage) Add(trigger Trigger) error {
	return couchdb.CreateDoc(couchdb.GlobalTriggersDB, trigger.Infos())
}

func (s *globalDBStorage) Delete(trigger Trigger) error {
	return couchdb.DeleteDoc(couchdb.GlobalTriggersDB, trigger.Infos())
}

// MemScheduler is a centralized scheduler of many triggers. It starts all of
// them and schedules jobs accordingly.
type MemScheduler struct {
	broker  jobs.Broker
	storage triggerGlobalStorage

	ts map[string]Trigger
	mu sync.RWMutex
}

// NewMemScheduler creates a new in-memory scheduler that will load all
// registered triggers and schedule their work.
func NewMemScheduler() *MemScheduler {
	return newMemScheduler(newGlobalDBStorage())
}

func newMemScheduler(storage triggerGlobalStorage) *MemScheduler {
	return &MemScheduler{
		storage: storage,
		ts:      make(map[string]Trigger),
	}
}

// Start will start the scheduler by actually loading all triggers from the
// scheduler's storage and associate for each of them a go routine in which
// they wait for the trigger send job requests.
func (s *MemScheduler) Start(b jobs.Broker) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ts, err := s.storage.GetAll()
	if err != nil {
		return err
	}
	s.broker = b
	for _, infos := range ts {
		t, err := NewTrigger(infos)
		if err != nil {
			log.Errorf("[jobs] scheduler: Could not load the trigger %s(%s) at startup: %s",
				infos.Type, infos.TID, err.Error())
			continue
		}
		s.ts[infos.TID] = t
		go s.schedule(t)
	}
	return nil
}

// Stop the scheduling of triggers
func (s *MemScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.ts {
		t.Unschedule()
	}
}

// Add will add a new trigger to the scheduler. The trigger is persisted in
// storage.
func (s *MemScheduler) Add(t Trigger) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.storage.Add(t); err != nil {
		return err
	}
	s.ts[t.Infos().TID] = t
	go s.schedule(t)
	return nil
}

// Get returns the trigger with the specified ID.
func (s *MemScheduler) Get(domain, id string) (Trigger, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.ts[id]
	if !ok || t.Infos().Domain != domain {
		return nil, ErrNotFoundTrigger
	}
	return t, nil
}

// Delete removes the trigger with the specified ID. The trigger is unscheduled
// and remove from the storage.
func (s *MemScheduler) Delete(domain, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.ts[id]
	if !ok || t.Infos().Domain != domain {
		return ErrNotFoundTrigger
	}
	if err := s.storage.Delete(t); err != nil {
		return err
	}
	delete(s.ts, id)
	t.Unschedule()
	return nil
}

// GetAll returns all the running in-memory triggers.
func (s *MemScheduler) GetAll(domain string) ([]Trigger, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v := make([]Trigger, 0)
	for _, t := range s.ts {
		if t.Infos().Domain == domain {
			v = append(v, t)
		}
	}
	return v, nil
}

func (s *MemScheduler) schedule(t Trigger) {
	log.Debugf("[jobs] trigger %s(%s): Starting trigger", t.Type(), t.Infos().TID)
	for req := range t.Schedule() {
		log.Debugf("[jobs] trigger %s(%s): Pushing new job", t.Type(), t.Infos().TID)
		if _, err := s.broker.PushJob(req); err != nil {
			log.Errorf("[jobs] trigger %s(%s): Could not schedule a new job: %s", t.Type(), t.Infos().TID, err.Error())
		}
	}
	log.Debugf("[jobs] trigger %s(%s): Closing trigger", t.Type(), t.Infos().TID)
	if err := s.Delete(t.Infos().Domain, t.Infos().TID); err != nil {
		log.Errorf("[jobs] trigger %s(%s): Could not delete trigger: %s", t.Type(), t.Infos().TID, err.Error())
	}
}

var _ Scheduler = &MemScheduler{}
