package latitude

import (
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/dnaeon/go-vcr/cassette"
	"github.com/dnaeon/go-vcr/recorder"
)

const (
	apiURLEnvVar           = "LATITUDE_API_URL"
	latitudeAccTestVar     = "LATITUDE_TEST_ACTUAL_API"
	testProjectPrefix      = "LATITUDE_TEST_PROJECT_"
	testPlanVar            = "LATITUDE_TEST_PLAN"
	testSiteVar            = "LATITUDE_TEST_SITE"
	testOperatingSystemVar = "LATITUDE_TEST_OS"
	testSSHKeyVar          = "LATITUDE_TEST_SSH_KEY"
	testRecorderEnv        = "LATITUDE_TEST_RECORDER"

	testRecorderRecord   = "record"
	testRecorderPlay     = "play"
	testRecorderDisabled = "disabled"
	recorderDefaultMode  = recorder.ModeDisabled

	// defaults should be available to most users
	testSiteDefault            = "ASH"
	testPlanDefault            = "c2-small-x86"
	testRegionDefault          = "ASH"
	testSSHKeyDefault          = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDQZtz6DPH4Y04vYLdOch5xOzDY7cdGWpYjBFx5H7ZzieVoRwartZAVTGX4qFT9aoyCuuE6qXYcTj6G1CdO5fb8iOtU6K3FdzVyw/WQ/c4sCehEL+wbYrOnXJSYMhLsUAFhZ69tTdmQSgctbv44yP32Z4xiE4zc/Bk465F3u4Zi1Jj883fyAgzahTWXOxpmvYAEuS6Qv6w4yJc6giiGFVYmu+N6h9j348UgbpToYiCSnSM4iNa9fs7sBGufOa9FuXtggPfXtpyk9f05AhkKEjPlCXcDNAq0GsvN2QEx3tYw6i5ze0qehv6EBAtwx3PLrj636O6IgSh0DgrZBih9NBov"
	testOperatingSystemDefault = "ubuntu_20_04_x64_lts"
)

func testPlan() string {
	envPlan := os.Getenv(testPlanVar)
	if envPlan != "" {
		return envPlan
	}
	return testPlanDefault
}

func testSite() string {
	envSite := os.Getenv(testSiteVar)
	if envSite != "" {
		return envSite
	}
	return testSiteDefault
}

func testOperatingSystem() string {
	envOS := os.Getenv(testOperatingSystemVar)
	if envOS != "" {
		return envOS
	}
	return testOperatingSystemDefault
}

func testSSHKey() string {
	envPlan := os.Getenv(testSSHKeyVar)
	if envPlan != "" {
		return envPlan
	}
	return testSSHKeyDefault
}

func randString8() string {
	// test recorder needs replayable names, not randoms
	mode, _ := testRecordMode()
	if mode != recorder.ModeDisabled {
		return "testrand"
	}

	n := 8
	rand.Seed(time.Now().UnixNano())
	letterRunes := []rune("acdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

// setupWithProject returns a client, project id, and teardown function
// configured for a new project with a test recorder for the named test
func setupWithProject(t *testing.T) (*Client, string, func()) {
	c, stopRecord := setup(t)
	rs := testProjectPrefix + randString8()
	pcr := ProjectCreateRequest{
		Data: ProjectCreateData{
			Type: testProjectType,
			Attributes: ProjectCreateAttributes{
				Name:        rs,
				Environment: testProjectEnvironment,
			},
		},
	}
	p, _, err := c.Projects.Create(&pcr)
	if err != nil {
		t.Fatal(err)
	}

	return c, p.Data.ID, func() {
		_, err := c.Projects.Delete(p.Data.ID)
		if err != nil {
			panic(fmt.Errorf("while deleting %s: %s", p.Data.Attributes.Name, err))
		}
		stopRecord()
	}
}

func setup(t *testing.T) (*Client, func()) {
	name := t.Name()
	apiToken := os.Getenv(authTokenEnvVar)
	if apiToken == "" {
		t.Fatalf("If you want to run latitude test, you must export %s.", authTokenEnvVar)
	}

	mode, err := testRecordMode()
	if err != nil {
		t.Fatal(err)
	}
	apiURL := os.Getenv(apiURLEnvVar)
	if apiURL == "" {
		apiURL = baseURL
	}
	r, stopRecord := testRecorder(t, name, mode)
	httpClient := http.DefaultClient
	httpClient.Transport = r
	c, err := NewClientWithBaseURL(apiToken, httpClient, apiURL)
	if err != nil {
		t.Fatal(err)
	}

	return c, stopRecord
}

func projectTeardown(c *Client) {
	ps, _, err := c.Projects.List(nil)
	if err != nil {
		panic(fmt.Errorf("while teardown: %s", err))
	}
	for _, p := range ps {
		fmt.Println(p.ID)
		if strings.HasPrefix(p.Attributes.Name, testProjectPrefix) {
			_, err := c.Projects.Delete(p.ID)
			if err != nil {
				panic(fmt.Errorf("while deleting %s: %s", p.Attributes.Name, err))
			}
		}
	}
}

func skipUnlessAcceptanceTestsAllowed(t *testing.T) {
	if os.Getenv(latitudeAccTestVar) == "" {
		t.Skipf("%s is not set", latitudeAccTestVar)
	}
}

func testRecordMode() (recorder.Mode, error) {
	modeRaw := os.Getenv(testRecorderEnv)
	mode := recorderDefaultMode

	switch strings.ToLower(modeRaw) {
	case testRecorderRecord:
		mode = recorder.ModeRecording
	case testRecorderPlay:
		mode = recorder.ModeReplaying
	case "":
		// no-op
	case testRecorderDisabled:
		// no-op
	default:
		return mode, fmt.Errorf("invalid %s mode: %s", testRecorderEnv, modeRaw)
	}
	return mode, nil
}

func testRecorder(t *testing.T, name string, mode recorder.Mode) (*recorder.Recorder, func()) {
	r, err := recorder.NewAsMode(path.Join("fixtures", name), mode, nil)
	if err != nil {
		t.Fatal(err)
	}

	r.AddFilter(func(i *cassette.Interaction) error {
		delete(i.Request.Headers, "X-Auth-Token")
		return nil
	})

	return r, func() {
		if err := r.Stop(); err != nil {
			t.Fatal(err)
		}
	}
}
