package features

import (
	"fmt"

	"github.com/cucumber/godog"
)

// socketBackendDoctorSteps are slice-42's steps. The Given steps pass the
// kernel's answer and the daemon's inventory name through unchanged to the
// T-66 fixture; the Then steps judge doctor's output and the harness's record.
func (w *world) socketBackendDoctorSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the kernel answers the Landlock version query with "([^"]*)"$`, w.pinKernelAnswer)
	sc.Step(`^the fixture daemon reports the runtime inventory "([^"]*)"$`, w.chooseRuntimeInventory)
	sc.Step(`^its JSON inspection reports no checkout socket backend$`, w.doctorNoSocketBackend)
	sc.Step(`^doctor asked the kernel for its Landlock ABI once and made no other Landlock call$`, w.doctorOneLandlockQuery)
}

// doctorNoSocketBackend requires the inspection to carry no checkout_sockets
// member at all, so a failed detection never leaks a backend.
func (w *world) doctorNoSocketBackend() error {
	object, err := w.doctorJSON()
	if err != nil {
		return err
	}
	inspection, ok := object["inspection"].(map[string]any)
	if !ok {
		return fmt.Errorf("JSON output has no inspection object\n%s", w.report())
	}
	if value, present := inspection["checkout_sockets"]; present {
		return fmt.Errorf("JSON inspection reports checkout sockets %#v, expected no member\n%s", value, w.report())
	}
	return nil
}

// doctorOneLandlockQuery requires the harness's record to hold exactly one
// supervised invocation, whose only call was the version query and received the
// pinned answer.
func (w *world) doctorOneLandlockQuery() error {
	record := w.landlockRecord()
	if len(record) != 1 || len(record[0]) != 1 {
		return fmt.Errorf("the harness's record is %v, expected one invocation with one Landlock call\n%s", record, w.report())
	}
	call, pinned := record[0][0], w.landlock.current()
	if !call.Version {
		return fmt.Errorf("doctor's one Landlock call was not the version query\n%s", w.report())
	}
	if call.Answer != pinned {
		return fmt.Errorf("doctor's Landlock query received %q, expected the pinned %q\n%s", call.Answer, pinned, w.report())
	}
	return nil
}
