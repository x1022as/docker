package opts

import (
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-units"
)

// ValidatorHugetlbType defines a validator function that returns a validated struct and/or an error.
type ValidatorHugetlbType func(val string) (container.Hugetlb, error)

// ValidateHugetlb validates that the specified string has a valid hugetlb format.
func ValidateHugetlb(htlb string) (container.Hugetlb, error) {
	var size, limit string
	var hugetlb container.Hugetlb

	ss := strings.Split(htlb, ":")
	if len(ss) == 1 {
		size = ""
		limit = ss[0]
	} else if len(ss) == 2 {
		if ss[0] == "" {
			size = ""
		} else {
			size = formatHugepageSize(ss[0])
		}
		limit = ss[1]
	} else {
		return hugetlb, fmt.Errorf("Invalid arguments for hugetlb-limit, too many colons")
	}

	ilimit, err := units.RAMInBytes(limit)
	if err != nil {
		return hugetlb, fmt.Errorf("Invalid hugetlb limit:%s", limit)
	}
	ulimit := uint64(ilimit)
	hugetlb = container.Hugetlb{
		PageSize: size,
		Limit:    ulimit,
	}
	return hugetlb, nil
}

// HugetlbOpt defines a map of Hugetlbs
type HugetlbOpt struct {
	values    []container.Hugetlb
	validator ValidatorHugetlbType
}

// NewHugetlbOpt creates a new HugetlbOpt
func NewHugetlbOpt(validator ValidatorHugetlbType) HugetlbOpt {
	values := []container.Hugetlb{}
	return HugetlbOpt{
		values:    values,
		validator: validator,
	}
}

// Set validates a Hugetlb and sets its name as a key in HugetlbOpt
func (opt *HugetlbOpt) Set(val string) error {
	var value container.Hugetlb
	if opt.validator != nil {
		v, err := opt.validator(val)
		if err != nil {
			return err
		}
		value = v
	}
	(opt.values) = append((opt.values), value)
	return nil
}

// String returns HugetlbOpt values as a string.
func (opt *HugetlbOpt) String() string {
	var out []string
	for _, v := range opt.values {
		out = append(out, fmt.Sprintf("%v", v))
	}

	return fmt.Sprintf("%v", out)
}

// GetList returns a slice of pointers to Hugetlbs.
func (opt *HugetlbOpt) GetAll() []container.Hugetlb {
	var hugetlbs []container.Hugetlb
	for _, v := range opt.values {
		hugetlbs = append(hugetlbs, v)
	}

	return hugetlbs
}

// Type returns the option type
func (opt *HugetlbOpt) Type() string {
	return "hugetlb"
}

func formatHugepageSize(s string) string {
	// make sure size get all 'b/k/m/g' replaced with "B/K/M/G"
	s = strings.ToUpper(s)
	// make sure size hase suffix "B"
	if !strings.HasSuffix(s, "B") {
		s = s + "B"
	}

	return s
}
