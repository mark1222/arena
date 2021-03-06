package commands

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Standalone Job Information
type StandaloneJob struct {
	*JobInfo
}

// Get Dashboard url of the job
func (sj *StandaloneJob) GetJobDashboards(client *kubernetes.Clientset) ([]string, error) {
	urls := []string{}
	// dashboardURL, err := dashboard(client, "kube-system", "kubernetes-dashboard")
	dashboardURL, err := dashboard(client, arenaNamespace, "kubernetes-dashboard")

	if err != nil {
		log.Debugf("Get dashboard failed due to %v", err)
		// retry for the existing customers, will be deprecated in the future
		dashboardURL, err = dashboard(client, "kube-system", "kubernetes-dashboard")
		if err != nil {
			log.Debugf("Get dashboard failed due to %v", err)
		}
	}

	if dashboardURL == "" {
		return urls, fmt.Errorf("No LOGVIEWER Installed.")
	}

	spec := sj.jobPod.Spec
	job := sj.job
	url := fmt.Sprintf("%s/#!/log/%s/%s/%s?namespace=%s\n",
		dashboardURL,
		job.Namespace,
		sj.jobPod.Name,
		spec.Containers[0].Name,
		job.Namespace)

	urls = append(urls, url)

	return urls, nil
}

// Standalone Job trainer
type StandaloneJobTrainer struct {
	client      *kubernetes.Clientset
	trainerType string
}

func NewStandaloneJobTrainer(client *kubernetes.Clientset) Trainer {

	log.Debugf("Init standalone job trainer")
	return &StandaloneJobTrainer{
		client:      client,
		trainerType: "standalonejob",
	}
}

func (s *StandaloneJobTrainer) Type() string {
	return s.trainerType
}

// check if it's Standalone job
func (s *StandaloneJobTrainer) IsSupported(name, ns string) bool {
	supported := false

	if len(allJobs) > 0 {
		for _, job := range allJobs {
			if isStandaloneJob(name, ns, job) {
				supported = true
				log.Debugf("the job %s for %s in namespace %s is found.", job.Name, name, ns)
				break
			}
		}
	} else {
		jobList, err := s.client.BatchV1().Jobs(namespace).List(metav1.ListOptions{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ListOptions",
				APIVersion: "v1",
			}, LabelSelector: fmt.Sprintf("release=%s", name),
		})
		if err != nil {
			log.Debugf("failed to search job %s in namespace %s due to %v", name, ns, err)
		}

		if len(jobList.Items) > 0 {
			supported = true
		}
	}

	return supported
}

func (s *StandaloneJobTrainer) GetTrainingJob(name, namespace string) (tj TrainingJob, err error) {
	if len(allPods) > 0 {
		tj, err = s.getTrainingJobFromCache(name, namespace)
	} else {
		tj, err = s.getTrainingJob(name, namespace)
	}

	return tj, err
}

func (s *StandaloneJobTrainer) getTrainingJob(name, namespace string) (TrainingJob, error) {
	var (
		jobPod v1.Pod
		job    batchv1.Job
		latest metav1.Time
	)

	// 1. Get the batchJob of trainig Job
	pods := []v1.Pod{}
	jobList, err := s.client.BatchV1().Jobs(namespace).List(metav1.ListOptions{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ListOptions",
			APIVersion: "v1",
		}, LabelSelector: fmt.Sprintf("release=%s", name),
	})
	if err != nil {
		return nil, err
	}

	if len(jobList.Items) == 0 {
		return nil, fmt.Errorf("Failed to find the job for %s", name)
	} else {
		job = jobList.Items[0]
	}

	// 2. Find the pod list, and determine the pod of the job
	podList, err := s.client.CoreV1().Pods(namespace).List(metav1.ListOptions{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ListOptions",
			APIVersion: "v1",
		}, LabelSelector: fmt.Sprintf("release=%s", name),
	})

	if err != nil {
		return nil, err
	}

	for _, item := range podList.Items {
		if jobPod.Name == "" {
			latest = item.CreationTimestamp
			jobPod = item
			log.Debugf("set pod %s as first jobpod, and it's time is %v", jobPod.Name, jobPod.CreationTimestamp)
		} else {
			log.Debugf("current jobpod %s , and it's time is %v", jobPod.Name, latest)
			log.Debugf("candidiate jobpod %s , and it's time is %v", item.Name, item.CreationTimestamp)
			current := item.CreationTimestamp
			if latest.Before(&current) {
				jobPod = item
				latest = current
				log.Debugf("replace")
			} else {
				log.Debugf("no replace")
			}
		}
	}

	pods = append(pods, jobPod)

	return &StandaloneJob{
		JobInfo: &JobInfo{
			job:         job,
			jobPod:      jobPod,
			pods:        pods,
			name:        name,
			trainerType: s.Type(),
		},
	}, nil

}

// get the training job from cache
func (s *StandaloneJobTrainer) getTrainingJobFromCache(name, ns string) (TrainingJob, error) {
	var (
		jobPod v1.Pod
		job    batchv1.Job
		latest metav1.Time
	)

	pods := []v1.Pod{}

	for _, item := range allJobs {
		if isStandaloneJob(name, ns, item) {
			job = item
			break
		}

	}

	for _, item := range allPods {
		if !isStandalonePod(name, ns, item) {
			continue
		}

		if jobPod.Name == "" {
			latest = item.CreationTimestamp
			jobPod = item
			log.Debugf("set pod %s as first jobpod, and it's time is %v", jobPod.Name, jobPod.CreationTimestamp)
		} else {
			log.Debugf("current jobpod %s , and it's time is %v", jobPod.Name, latest)
			log.Debugf("candidiate jobpod %s , and it's time is %v", item.Name, item.CreationTimestamp)
			current := item.CreationTimestamp
			if latest.Before(&current) {
				jobPod = item
				latest = current
				log.Debugf("replace")
			} else {
				log.Debugf("no replace")
			}
		}

	}

	pods = append(pods, jobPod)

	return &StandaloneJob{
		JobInfo: &JobInfo{
			job:         job,
			jobPod:      jobPod,
			pods:        pods,
			name:        name,
			trainerType: s.Type(),
		},
	}, nil
}

func isStandaloneJob(name, ns string, item batchv1.Job) bool {

	if val, ok := item.Labels["release"]; ok && (val == name) {
		log.Debugf("the job %s with labels %s", item.Name, val)
	} else {
		return false
	}

	if val, ok := item.Labels["app"]; ok && (val == "training") {
		log.Debugf("the job %s with labels %s is found.", item.Name, val)
	} else {
		return false
	}

	if item.Namespace != ns {
		return false
	}
	return true
}

func isStandalonePod(name, ns string, item v1.Pod) bool {
	if val, ok := item.Labels["release"]; ok && (val == name) {
		log.Debugf("the pod %s with labels %s", item.Name, val)
	} else {
		return false
	}

	if val, ok := item.Labels["app"]; ok && (val == "training") {
		log.Debugf("the pod %s with labels %s is found.", item.Name, val)
	} else {
		return false
	}

	if item.Namespace != ns {
		return false
	}
	return true
}
