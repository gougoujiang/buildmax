package harbor

import (
	"fmt"
	"time"

	"github.com/gougoujiang/buildmax/evaluation/contract"
)

// Options is everything an import needs beyond the job directory and the pins.
type Options struct {
	// Subject is what the job cannot say about what it measured.
	Subject SubjectInput
	// ExperimentID names the measurement these bundles belong to.
	ExperimentID string
	// CreatedAt dates the experiment. It is passed rather than read from the
	// clock so an import is reproducible, and so a caller can date the record
	// from the job it read rather than from when it got round to reading it.
	CreatedAt time.Time
	// Retention is how much of the subject's free text the bundles keep. Empty
	// means bounded, which is what a local diagnosis wants; an export lowers it.
	Retention contract.RetentionLevel
}

func (o Options) retention() contract.RetentionLevel {
	if o.Retention == "" {
		return contract.RetentionBounded
	}
	return o.Retention
}

// Import reads a finished Harbor job and writes it into a BuildMax bundle tree.
//
// It is one direction only. Harbor ran the benchmark, its verifier decided each
// outcome, and its job directory keeps the trajectories and artifacts; this
// copies none of that and rewrites none of it. What it produces is the record
// that makes an external result comparable with a BuildMax one: the subject
// tuple a qualification has to name, and one bundle per attempt carrying the
// verdict, the failure class, the usage, and the path back to the evidence.
func Import(jobDir, bundleRoot string, pins Pins, opt Options) (Conversion, error) {
	trials, err := LoadJob(jobDir)
	if err != nil {
		return Conversion{}, err
	}
	conversion, err := Convert(trials, pins, opt)
	if err != nil {
		return Conversion{}, err
	}

	for _, bundle := range conversion.Bundles {
		if _, err := contract.WriteBundle(bundleRoot, bundle); err != nil {
			return Conversion{}, fmt.Errorf("write bundle for %s: %w", bundle.TaskID, err)
		}
	}

	experiment := contract.Experiment{
		ID:        opt.ExperimentID,
		Name:      "harbor import of " + jobDir,
		CreatedAt: opt.CreatedAt,
		Dataset:   conversion.Subject.Dataset,
		Subjects:  []contract.SubjectManifest{conversion.Subject},
		Trials:    conversion.Attempts(),
		Tasks:     conversion.Tasks(),
	}
	if err := contract.WriteExperiment(bundleRoot, experiment); err != nil {
		return Conversion{}, err
	}
	return conversion, nil
}

// Tasks returns the task ids the job covered, in order and without repeats.
func (c Conversion) Tasks() []string {
	seen := map[string]bool{}
	var tasks []string
	for _, b := range c.Bundles {
		if seen[b.TaskID] {
			continue
		}
		seen[b.TaskID] = true
		tasks = append(tasks, b.TaskID)
	}
	return tasks
}

// Attempts returns the highest attempt count any task reached.
//
// The maximum rather than the mean: Harbor retries a trial that failed for a
// harness reason, so a job can hold more attempts for one task than another,
// and reporting an average would describe a repetition count no task actually
// ran.
func (c Conversion) Attempts() int {
	counts := map[string]int{}
	most := 0
	for _, b := range c.Bundles {
		counts[b.TaskID]++
		if counts[b.TaskID] > most {
			most = counts[b.TaskID]
		}
	}
	return most
}
