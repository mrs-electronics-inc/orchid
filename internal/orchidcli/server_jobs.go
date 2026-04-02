package orchidcli

import (
	"fmt"
	"log"
	"sync"
)

type daemonJob struct {
	mu     sync.Mutex
	status daemonJobStatus
}

type daemonJobStore struct {
	mu   sync.Mutex
	next int64
	jobs map[string]*daemonJob
}

func newDaemonJobStore() *daemonJobStore {
	return &daemonJobStore{jobs: make(map[string]*daemonJob)}
}

func (s *daemonJobStore) create(initial daemonJobStatus) *daemonJob {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.next++
	jobID := fmt.Sprintf("job-%d", s.next)
	job := &daemonJob{
		status: daemonJobStatus{
			JobID:   jobID,
			State:   daemonJobStateQueued,
			Stage:   initial.Stage,
			Message: initial.Message,
			VMName:  initial.VMName,
			IP:      initial.IP,
			Error:   initial.Error,
		},
	}
	s.jobs[jobID] = job
	log.Printf("job %s created state=%s stage=%s vm=%s message=%q", jobID, job.status.State, job.status.Stage, job.status.VMName, job.status.Message)
	return job
}

func (s *daemonJobStore) get(jobID string) (*daemonJob, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[jobID]
	return job, ok
}

func (j *daemonJob) snapshot() daemonJobStatus {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.status
}

func (j *daemonJob) update(state, stage, message, vmName, ip string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if state != "" {
		j.status.State = state
	}
	if stage != "" {
		j.status.Stage = stage
	}
	if message != "" {
		j.status.Message = message
	}
	if vmName != "" {
		j.status.VMName = vmName
	}
	if ip != "" {
		j.status.IP = ip
	}
	if state == daemonJobStateSucceeded {
		j.status.Error = ""
	}
	log.Printf("job %s state=%s stage=%s vm=%s ip=%s message=%q", j.status.JobID, j.status.State, j.status.Stage, j.status.VMName, j.status.IP, j.status.Message)
}

func (j *daemonJob) fail(stage, message, errText string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.status.State = daemonJobStateFailed
	if stage != "" {
		j.status.Stage = stage
	}
	if message != "" {
		j.status.Message = message
	}
	j.status.Error = errText
	log.Printf("job %s failed state=%s stage=%s vm=%s ip=%s message=%q error=%q", j.status.JobID, j.status.State, j.status.Stage, j.status.VMName, j.status.IP, j.status.Message, j.status.Error)
}
