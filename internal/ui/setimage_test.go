package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var testContainers = []msgs.ContainerImageChange{
	{Name: "nginx", Image: "nginx:1.25", Init: false},
	{Name: "sidecar", Image: "envoy:v1.28", Init: false},
}

var testGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

// tabToButtons presses Tab until the button bar is focused, returning the
// updated overlay. It guards against an infinite loop.
func tabToButtons(t *testing.T, si SetImageOverlay) SetImageOverlay {
	t.Helper()
	for i := 0; i < 100; i++ {
		if si.focusKind == focusButtons {
			return si
		}
		si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	}
	t.Fatal("never reached button focus after 100 Tabs")
	return si
}

func TestSetImageOverlayOpen(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)
	if !si.Active() {
		t.Fatal("expected overlay to be active after Open")
	}
	if si.overlay.InputCount() != 2 {
		t.Fatalf("expected 2 inputs, got %d", si.overlay.InputCount())
	}
	if si.overlay.InputValue(0) != "nginx:1.25" {
		t.Fatalf("expected 'nginx:1.25', got %q", si.overlay.InputValue(0))
	}
	if si.overlay.InputValue(1) != "envoy:v1.28" {
		t.Fatalf("expected 'envoy:v1.28', got %q", si.overlay.InputValue(1))
	}
	if !si.InputFocused() {
		t.Fatal("expected input to be focused after Open")
	}
	if si.FocusedButton() != setImageBtnYes {
		t.Fatal("expected Yes button focused by default")
	}
}

func TestSetImageOverlayEscape(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if si.Active() {
		t.Fatal("expected overlay to be inactive after Escape")
	}
}

func TestSetImageOverlaySubmitChanged(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)

	si.overlay.Input(0).SetValue("nginx:1.26")

	si, cmd := si.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if si.Active() {
		t.Fatal("expected overlay to be inactive after submit")
	}
	if cmd == nil {
		t.Fatal("expected a command to be returned")
	}
	msg := cmd()
	siMsg, ok := msg.(msgs.SetImageRequestedMsg)
	if !ok {
		t.Fatalf("expected SetImageRequestedMsg, got %T", msg)
	}
	if len(siMsg.Images) != 1 {
		t.Fatalf("expected 1 changed image, got %d", len(siMsg.Images))
	}
	if siMsg.Images[0].Name != "nginx" || siMsg.Images[0].Image != "nginx:1.26" {
		t.Errorf("unexpected image change: %+v", siMsg.Images[0])
	}
	if siMsg.Images[0].PullPolicy != "" {
		t.Errorf("expected empty PullPolicy for image-only change, got %q", siMsg.Images[0].PullPolicy)
	}
	if siMsg.ResourceName != "my-deploy" {
		t.Errorf("expected resource name 'my-deploy', got %q", siMsg.ResourceName)
	}
}

func TestSetImageOverlaySubmitUnchanged(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)

	si, cmd := si.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if si.Active() {
		t.Fatal("expected overlay to close on unchanged submit")
	}
	if cmd != nil {
		t.Fatal("expected no command when nothing changed")
	}
}

func TestSetImageOverlaySubmitClearedImageNoCommand(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)

	// Clear row 0's image entirely; nothing else changes.
	si.overlay.Input(0).SetValue("")

	si, cmd := si.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if si.Active() {
		t.Fatal("expected overlay to close")
	}
	if cmd != nil {
		t.Fatal("expected NO command when the image field is emptied (clear is not a valid change)")
	}
}

func TestSetImageOverlayEmptyContainersNoPanic(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", nil)

	// None of these key presses may panic on a zero-container overlay.
	for _, km := range []tea.KeyPressMsg{
		{Code: tea.KeySpace, Text: " "},
		{Code: tea.KeyLeft},
		{Code: tea.KeyRight},
		{Code: tea.KeyTab},
		{Code: tea.KeyTab, Mod: tea.ModShift},
		{Code: tea.KeyUp},
		{Code: tea.KeyDown},
	} {
		si, _ = si.Update(km)
	}
	if !si.Active() {
		t.Fatal("expected overlay to remain active after non-Escape keys")
	}

	// Escape must still close even with no rows.
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if si.Active() {
		t.Fatal("expected overlay to close on Escape with no containers")
	}
}

func TestSetImageOverlayTabCycle(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)

	if si.overlay.FocusedInput() != 0 {
		t.Fatalf("expected input 0 focused, got %d", si.overlay.FocusedInput())
	}

	// Tab order: img0 -> pull0 -> img1 -> pull1 -> buttons -> (wrap) img0.

	// Tab: img0 -> pull0
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if si.focusKind != focusPolicy || si.focusRow != 0 {
		t.Fatalf("expected policy 0 focused, got kind=%d row=%d", si.focusKind, si.focusRow)
	}

	// Tab: pull0 -> img1
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if !si.InputFocused() || si.overlay.FocusedInput() != 1 {
		t.Fatalf("expected image input 1 focused, got kind=%d input=%d", si.focusKind, si.overlay.FocusedInput())
	}

	// Tab: img1 -> pull1
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if si.focusKind != focusPolicy || si.focusRow != 1 {
		t.Fatalf("expected policy 1 focused, got kind=%d row=%d", si.focusKind, si.focusRow)
	}

	// Tab: pull1 (last) -> buttons
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if si.InputFocused() || si.focusKind != focusButtons {
		t.Fatalf("expected buttons focused after Tab from last policy, got kind=%d", si.focusKind)
	}

	// Tab: buttons -> img0 (wrap)
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if !si.InputFocused() {
		t.Fatal("expected image input focused after Tab from buttons")
	}
	if si.overlay.FocusedInput() != 0 {
		t.Fatalf("expected input 0 focused after wrap, got %d", si.overlay.FocusedInput())
	}
}

func TestSetImageOverlaySingleContainer(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-pod", "default", testGVR, "pods", testContainers[:1])
	if si.overlay.InputCount() != 1 {
		t.Fatalf("expected 1 input, got %d", si.overlay.InputCount())
	}
}

func TestSetImageOverlaySingleContainerTabCycle(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-pod", "default", testGVR, "pods", testContainers[:1])

	// Order: img0 -> pull0 -> buttons -> (wrap) img0.

	// Tab: img0 -> pull0
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if si.focusKind != focusPolicy {
		t.Fatalf("expected policy focused after Tab from only input, got kind=%d", si.focusKind)
	}

	// Tab: pull0 (last) -> buttons
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if si.InputFocused() || si.focusKind != focusButtons {
		t.Fatalf("expected buttons focused, got kind=%d", si.focusKind)
	}

	// Tab: buttons -> img0
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if !si.InputFocused() {
		t.Fatal("expected input focused after Tab from buttons")
	}
	if si.overlay.FocusedInput() != 0 {
		t.Fatalf("expected input 0 focused, got %d", si.overlay.FocusedInput())
	}
}

func TestSetImageOverlayInitContainer(t *testing.T) {
	containers := []msgs.ContainerImageChange{
		{Name: "app", Image: "myapp:v1", Init: false},
		{Name: "init-db", Image: "busybox:1.0", Init: true},
	}
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", containers)

	si.overlay.Input(1).SetValue("busybox:2.0")

	si, cmd := si.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command")
	}
	msg := cmd().(msgs.SetImageRequestedMsg)
	if len(msg.Images) != 1 {
		t.Fatalf("expected 1 changed image, got %d", len(msg.Images))
	}
	if !msg.Images[0].Init {
		t.Fatal("expected init flag to be preserved")
	}
}

func TestSetImageOverlayView(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)
	view := si.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
	// The body must include the per-container policy widget. If the
	// body-composition / no-auto-input render path breaks, the policy prompt
	// vanishes — assert its presence to catch that.
	if !strings.Contains(view, "pull:") {
		t.Fatalf("expected rendered view to contain the policy prompt %q, got:\n%s", "pull:", view)
	}
}

// ── Focus and button tests ──

func TestSetImageOverlayTabTogglesFocus(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)

	if !si.InputFocused() {
		t.Fatal("expected input focused initially")
	}

	// Tab through all fields to the buttons.
	si = tabToButtons(t, si)
	if si.InputFocused() {
		t.Fatal("expected buttons focused after Tab to button bar")
	}

	// Tab from buttons back to input 0.
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if !si.InputFocused() {
		t.Fatal("expected input focused after Tab from buttons")
	}
	if si.overlay.FocusedInput() != 0 {
		t.Fatalf("expected input 0 focused, got %d", si.overlay.FocusedInput())
	}
}

func TestSetImageOverlayButtonHotkeys(t *testing.T) {
	// Test 'y' hotkey submits.
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)
	si.overlay.Input(0).SetValue("nginx:1.26")

	// Tab to buttons.
	si = tabToButtons(t, si)
	si, cmd := si.Update(tea.KeyPressMsg{Code: -1, Text: "y"})
	if si.Active() {
		t.Fatal("expected overlay to close after y hotkey")
	}
	if cmd == nil {
		t.Fatal("expected a command from y hotkey")
	}
	msg := cmd()
	siMsg, ok := msg.(msgs.SetImageRequestedMsg)
	if !ok {
		t.Fatalf("expected SetImageRequestedMsg, got %T", msg)
	}
	if len(siMsg.Images) != 1 || siMsg.Images[0].Image != "nginx:1.26" {
		t.Fatalf("unexpected image change: %+v", siMsg.Images)
	}

	// Test 'n' hotkey closes without command.
	si2 := NewSetImageOverlay(80, 30)
	si2.Open("my-deploy", "default", testGVR, "deployments", testContainers)

	si2 = tabToButtons(t, si2)
	si2, cmd2 := si2.Update(tea.KeyPressMsg{Code: -1, Text: "n"})
	if si2.Active() {
		t.Fatal("expected overlay to close after n hotkey")
	}
	if cmd2 != nil {
		t.Fatal("expected no command from n hotkey (cancel)")
	}
}

func TestSetImageOverlayHotkeysIgnoredWhenInputFocused(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)

	// Input is focused — y/n should go to the input, not act as hotkeys.
	si, _ = si.Update(tea.KeyPressMsg{Code: -1, Text: "y"})
	if !si.Active() {
		t.Fatal("overlay should remain active when y typed into input")
	}

	si, _ = si.Update(tea.KeyPressMsg{Code: -1, Text: "n"})
	if !si.Active() {
		t.Fatal("overlay should remain active when n typed into input")
	}
}

func TestSetImageOverlayButtonNavigation(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)

	// Tab to buttons — Yes is focused by default.
	si = tabToButtons(t, si)
	if si.FocusedButton() != setImageBtnYes {
		t.Fatal("expected Yes button focused")
	}

	// Right arrow -> No.
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if si.FocusedButton() != setImageBtnNo {
		t.Fatal("expected No button focused after Right")
	}

	// Left arrow -> Yes.
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if si.FocusedButton() != setImageBtnYes {
		t.Fatal("expected Yes button focused after Left")
	}
}

func TestSetImageOverlayArrowsMoveCursorWhenInputFocused(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)

	// Input 0 is focused after Open. Place the cursor at the end of the value.
	si.overlay.Input(0).CursorEnd()
	end := si.overlay.Input(0).Position()
	if end == 0 {
		t.Fatal("expected non-zero cursor position at end of input")
	}

	// Left arrow must move the cursor within the input, not switch buttons.
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if !si.InputFocused() {
		t.Fatal("expected input to remain focused after Left")
	}
	if got := si.overlay.Input(0).Position(); got != end-1 {
		t.Fatalf("expected cursor at %d after Left, got %d", end-1, got)
	}

	// Right arrow must move the cursor back toward the end.
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if got := si.overlay.Input(0).Position(); got != end {
		t.Fatalf("expected cursor at %d after Right, got %d", end, got)
	}
}

func TestSetImageOverlaySubwordMotion(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)

	// Input 0 holds "nginx:1.25" and is focused. Start at the end.
	si.overlay.Input(0).CursorEnd()

	// alt+b walks back over sub-word segments: 25 . 1 : nginx.
	for _, want := range []int{8, 7, 6, 5, 0} {
		si, _ = si.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModAlt})
		if got := si.overlay.Input(0).Position(); got != want {
			t.Fatalf("alt+b: expected cursor %d, got %d", want, got)
		}
	}

	// alt+right walks forward over the same segments (word-forward alias).
	for _, want := range []int{5, 6, 7, 8, 10} {
		si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModAlt})
		if got := si.overlay.Input(0).Position(); got != want {
			t.Fatalf("alt+right: expected cursor %d, got %d", want, got)
		}
	}
}

func TestSetImageOverlaySubwordDelete(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)

	// Input 0 holds "nginx:1.25", cursor at end.
	si.overlay.Input(0).CursorEnd()

	// alt+backspace deletes one sub-word segment at a time.
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyBackspace, Mod: tea.ModAlt})
	if got := si.overlay.InputValue(0); got != "nginx:1." {
		t.Fatalf("after alt+backspace: expected %q, got %q", "nginx:1.", got)
	}
	if got := si.overlay.Input(0).Position(); got != 8 {
		t.Fatalf("after alt+backspace: expected cursor 8, got %d", got)
	}

	// ctrl+w deletes the next segment back ("." here).
	si, _ = si.Update(tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl})
	if got := si.overlay.InputValue(0); got != "nginx:1" {
		t.Fatalf("after ctrl+w: expected %q, got %q", "nginx:1", got)
	}
}

func TestSetImageOverlayEnterOnNoButton(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)

	// Tab to buttons, navigate to No, press Enter.
	si = tabToButtons(t, si)
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	si, cmd := si.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if si.Active() {
		t.Fatal("expected overlay to close after Enter on No")
	}
	if cmd != nil {
		t.Fatal("expected no command from No button")
	}
}

func TestSetImageOverlayUpFromButtonsFocusesLastPolicy(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)

	// Tab to buttons.
	si = tabToButtons(t, si)
	if si.focusKind != focusButtons {
		t.Fatal("expected buttons focused")
	}

	// Up arrow lands on the LAST container's policy cycle.
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if si.focusKind != focusPolicy {
		t.Fatalf("expected policy focused after Up from buttons, got kind=%d", si.focusKind)
	}
	if si.focusRow != len(testContainers)-1 {
		t.Fatalf("expected last container (%d) policy focused, got row %d", len(testContainers)-1, si.focusRow)
	}
	if !si.rows[si.focusRow].policy.Focused() {
		t.Fatal("expected the last policy field's Focused() to be true")
	}
}

// ── Pull-policy cycle tests ──

func TestSetImageOverlayOpenSeedsPolicy(t *testing.T) {
	containers := []msgs.ContainerImageChange{
		{Name: "nginx", Image: "nginx:1.25", PullPolicy: "IfNotPresent"},
		{Name: "sidecar", Image: "envoy:v1.28"}, // unset -> "(default)"
	}
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", containers)

	if len(si.rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(si.rows))
	}
	if got := si.rows[0].policy.Value(); got != "IfNotPresent" {
		t.Fatalf("expected row 0 policy 'IfNotPresent', got %q", got)
	}
	if got := si.rows[1].policy.Value(); got != "" {
		t.Fatalf("expected row 1 policy default (\"\"), got %q", got)
	}
}

func TestSetImageOverlayFullTabOrder(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)

	type step struct {
		kind setImageFocus
		row  int // image input index / policy row; ignored for buttons
	}
	want := []step{
		{focusImage, 0},  // initial
		{focusPolicy, 0}, // Tab
		{focusImage, 1},
		{focusPolicy, 1},
		{focusButtons, 0},
		{focusImage, 0}, // wrap
	}

	// Verify initial focus.
	if si.focusKind != want[0].kind || si.focusRow != want[0].row {
		t.Fatalf("initial: expected kind=%d row=%d, got kind=%d row=%d", want[0].kind, want[0].row, si.focusKind, si.focusRow)
	}

	for i := 1; i < len(want); i++ {
		si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		w := want[i]
		if si.focusKind != w.kind {
			t.Fatalf("step %d: expected kind=%d, got %d", i, w.kind, si.focusKind)
		}
		switch w.kind {
		case focusImage:
			if si.overlay.FocusedInput() != w.row {
				t.Fatalf("step %d: expected image input %d, got %d", i, w.row, si.overlay.FocusedInput())
			}
		case focusPolicy:
			if si.focusRow != w.row {
				t.Fatalf("step %d: expected policy row %d, got %d", i, w.row, si.focusRow)
			}
			if !si.rows[w.row].policy.Focused() {
				t.Fatalf("step %d: expected policy row %d focused", i, w.row)
			}
		}
	}
}

// focusPolicyRow Tabs from the just-opened overlay until the given row's
// policy cycle is focused, mirroring real keyboard navigation.
func focusPolicyRow(t *testing.T, si SetImageOverlay, row int) SetImageOverlay {
	t.Helper()
	for i := 0; i < 100; i++ {
		if si.focusKind == focusPolicy && si.focusRow == row {
			return si
		}
		si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	}
	t.Fatalf("never reached policy row %d after 100 Tabs", row)
	return si
}

func TestSetImageOverlaySubmitPolicyOnly(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)

	// Focus row 0's policy and cycle (default) -> Always.
	si = focusPolicyRow(t, si, 0)
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyRight})

	si, cmd := si.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command for a policy-only change")
	}
	msg := cmd().(msgs.SetImageRequestedMsg)
	if len(msg.Images) != 1 {
		t.Fatalf("expected 1 change, got %d", len(msg.Images))
	}
	got := msg.Images[0]
	if got.Name != "nginx" {
		t.Errorf("expected name 'nginx', got %q", got.Name)
	}
	if got.Image != "" {
		t.Errorf("expected empty Image for policy-only change, got %q", got.Image)
	}
	if got.PullPolicy != "Always" {
		t.Errorf("expected PullPolicy 'Always', got %q", got.PullPolicy)
	}
	if got.Init {
		t.Errorf("expected Init false preserved, got %v", got.Init)
	}
}

func TestSetImageOverlaySubmitPolicyOnlyPreservesInit(t *testing.T) {
	containers := []msgs.ContainerImageChange{
		{Name: "init-db", Image: "busybox:1.0", Init: true},
	}
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", containers)

	si = focusPolicyRow(t, si, 0)
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyRight}) // -> Always

	si, cmd := si.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command")
	}
	msg := cmd().(msgs.SetImageRequestedMsg)
	if len(msg.Images) != 1 {
		t.Fatalf("expected 1 change, got %d", len(msg.Images))
	}
	if !msg.Images[0].Init {
		t.Error("expected Init flag preserved on policy-only change")
	}
	if msg.Images[0].Image != "" {
		t.Errorf("expected empty Image, got %q", msg.Images[0].Image)
	}
}

func TestSetImageOverlaySubmitImageOnlyHasEmptyPolicy(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)

	si.overlay.Input(0).SetValue("nginx:1.26")

	si, cmd := si.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command for an image-only change")
	}
	msg := cmd().(msgs.SetImageRequestedMsg)
	if len(msg.Images) != 1 {
		t.Fatalf("expected 1 change, got %d", len(msg.Images))
	}
	got := msg.Images[0]
	if got.Image != "nginx:1.26" {
		t.Errorf("expected Image 'nginx:1.26', got %q", got.Image)
	}
	if got.PullPolicy != "" {
		t.Errorf("expected empty PullPolicy for image-only change, got %q", got.PullPolicy)
	}
}

func TestSetImageOverlaySubmitImageAndPolicy(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)

	si.overlay.Input(0).SetValue("nginx:1.26")

	si = focusPolicyRow(t, si, 0)
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyRight}) // (default) -> Always

	si, cmd := si.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command when both image and policy changed")
	}
	msg := cmd().(msgs.SetImageRequestedMsg)
	if len(msg.Images) != 1 {
		t.Fatalf("expected 1 change, got %d", len(msg.Images))
	}
	got := msg.Images[0]
	if got.Image != "nginx:1.26" {
		t.Errorf("expected Image 'nginx:1.26', got %q", got.Image)
	}
	if got.PullPolicy != "Always" {
		t.Errorf("expected PullPolicy 'Always', got %q", got.PullPolicy)
	}
}

func TestSetImageOverlaySubmitMultiContainerIndependentDiff(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)

	// Row 0: change the image only.
	si.overlay.Input(0).SetValue("nginx:1.26")

	// Row 1: change the policy only (default -> Always).
	si = focusPolicyRow(t, si, 1)
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyRight})

	si, cmd := si.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command for independent multi-container changes")
	}
	msg := cmd().(msgs.SetImageRequestedMsg)
	if len(msg.Images) != 2 {
		t.Fatalf("expected 2 changes, got %d: %+v", len(msg.Images), msg.Images)
	}

	byName := map[string]msgs.ContainerImageChange{}
	for _, c := range msg.Images {
		byName[c.Name] = c
	}

	nginx, ok := byName["nginx"]
	if !ok {
		t.Fatal("expected a change for 'nginx'")
	}
	if nginx.Image != "nginx:1.26" {
		t.Errorf("nginx: expected Image 'nginx:1.26', got %q", nginx.Image)
	}
	if nginx.PullPolicy != "" {
		t.Errorf("nginx: expected empty PullPolicy (image-only), got %q", nginx.PullPolicy)
	}

	sidecar, ok := byName["sidecar"]
	if !ok {
		t.Fatal("expected a change for 'sidecar'")
	}
	if sidecar.Image != "" {
		t.Errorf("sidecar: expected empty Image (policy-only), got %q", sidecar.Image)
	}
	if sidecar.PullPolicy != "Always" {
		t.Errorf("sidecar: expected PullPolicy 'Always', got %q", sidecar.PullPolicy)
	}
}

func TestSetImageOverlaySubmitMultiContainerExcludesUnchanged(t *testing.T) {
	containers := []msgs.ContainerImageChange{
		{Name: "nginx", Image: "nginx:1.25"},
		{Name: "sidecar", Image: "envoy:v1.28"},
		{Name: "logger", Image: "fluentd:v1.16"},
	}
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", containers)

	// Change only the middle container; the other two are untouched.
	si.overlay.Input(1).SetValue("envoy:v1.29")

	si, cmd := si.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command")
	}
	msg := cmd().(msgs.SetImageRequestedMsg)
	if len(msg.Images) != 1 {
		t.Fatalf("expected only the changed container, got %d: %+v", len(msg.Images), msg.Images)
	}
	if msg.Images[0].Name != "sidecar" || msg.Images[0].Image != "envoy:v1.29" {
		t.Errorf("unexpected change: %+v", msg.Images[0])
	}
}

func TestSetImageOverlaySubmitPolicyRevertToDefaultSuppressed(t *testing.T) {
	containers := []msgs.ContainerImageChange{
		{Name: "nginx", Image: "nginx:1.25", PullPolicy: "Always"},
	}
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", containers)

	// Orig policy is "Always" (index 1). Cycle back to (default).
	si = focusPolicyRow(t, si, 0)
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyLeft}) // Always -> (default)
	if got := si.rows[0].policy.Value(); got != "" {
		t.Fatalf("expected policy at default, got %q", got)
	}

	si, cmd := si.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if si.Active() {
		t.Fatal("expected overlay to close")
	}
	if cmd != nil {
		t.Fatal("expected NO command when reverting policy to default (suppressed)")
	}
}

func TestSetImageOverlaySubmitDefaultUntouchedNoCommand(t *testing.T) {
	// Orig policy unset, image unchanged, nothing touched -> no command.
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)

	si, cmd := si.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if si.Active() {
		t.Fatal("expected overlay to close")
	}
	if cmd != nil {
		t.Fatal("expected no command when nothing changed")
	}
}

func TestSetImageOverlayPolicyCycleDoesNotLeak(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)

	// Focus the first policy cycle (img0 -> pull0).
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if si.focusKind != focusPolicy || si.focusRow != 0 {
		t.Fatalf("expected policy 0 focused, got kind=%d row=%d", si.focusKind, si.focusRow)
	}

	// Record the image input cursor position; policy keys must not touch it.
	startPos := si.overlay.Input(0).Position()

	// Right cycles forward: (default) -> Always.
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if got := si.rows[0].policy.Value(); got != "Always" {
		t.Fatalf("after Right: expected 'Always', got %q", got)
	}
	// Right must NOT move to the No button.
	if si.focusKind != focusPolicy {
		t.Fatal("Right leaked out of policy cycle into button navigation")
	}
	if si.FocusedButton() != setImageBtnYes {
		t.Fatalf("Right changed focused button to %d", si.FocusedButton())
	}

	// Left cycles back: Always -> (default).
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if got := si.rows[0].policy.Value(); got != "" {
		t.Fatalf("after Left: expected default (\"\"), got %q", got)
	}
	if si.focusKind != focusPolicy {
		t.Fatal("Left leaked out of policy cycle into button navigation")
	}

	// Space cycles forward: (default) -> Always.
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if got := si.rows[0].policy.Value(); got != "Always" {
		t.Fatalf("after Space: expected 'Always', got %q", got)
	}
	if si.focusKind != focusPolicy {
		t.Fatal("Space leaked out of policy cycle")
	}

	// The image input cursor must be untouched by policy-cycle keys.
	if got := si.overlay.Input(0).Position(); got != startPos {
		t.Fatalf("policy keys moved image cursor: expected %d, got %d", startPos, got)
	}
	// And the image value must be unchanged (Space did not type into input).
	if got := si.overlay.InputValue(0); got != "nginx:1.25" {
		t.Fatalf("policy keys mutated image value: got %q", got)
	}
}

// ── Backward navigation (Up / Shift+Tab) ──

// TestSetImageOverlayUpBackwardWalk verifies the reverse focus order:
// pull(r) -> img(r), img(r) -> pull(r-1), and img(0) -> buttons (wrap).
func TestSetImageOverlayUpBackwardWalk(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)

	// Move to row 1's policy via Tab (img0 -> pull0 -> img1 -> pull1).
	si = focusPolicyRow(t, si, 1)
	if si.focusKind != focusPolicy || si.focusRow != 1 {
		t.Fatalf("precondition: expected policy row 1, got kind=%d row=%d", si.focusKind, si.focusRow)
	}

	// Up: pull1 -> img1 (same row's image input).
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if !si.InputFocused() || si.overlay.FocusedInput() != 1 {
		t.Fatalf("Up from policy row 1 should focus image input 1, got kind=%d input=%d", si.focusKind, si.overlay.FocusedInput())
	}

	// Up: img1 -> pull0 (previous row's policy).
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if si.focusKind != focusPolicy || si.focusRow != 0 {
		t.Fatalf("Up from image row 1 should focus policy row 0, got kind=%d row=%d", si.focusKind, si.focusRow)
	}
	if !si.rows[0].policy.Focused() {
		t.Fatal("expected policy row 0 Focused() true after Up")
	}

	// Up: pull0 -> img0 (same row's image input).
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if !si.InputFocused() || si.overlay.FocusedInput() != 0 {
		t.Fatalf("Up from policy row 0 should focus image input 0, got kind=%d input=%d", si.focusKind, si.overlay.FocusedInput())
	}

	// Up: img0 (first) -> buttons (wrap).
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if si.focusKind != focusButtons {
		t.Fatalf("Up from first image should wrap to buttons, got kind=%d", si.focusKind)
	}
}

// TestSetImageOverlayShiftTabWalksBackward verifies Shift+Tab moves focus
// backward through the same order as Up.
func TestSetImageOverlayShiftTabWalksBackward(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)

	shiftTab := tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}

	// From buttons, Shift+Tab walks back to the last policy.
	si = tabToButtons(t, si)
	si, _ = si.Update(shiftTab)
	if si.focusKind != focusPolicy || si.focusRow != len(testContainers)-1 {
		t.Fatalf("Shift+Tab from buttons should focus last policy, got kind=%d row=%d", si.focusKind, si.focusRow)
	}

	// Walk all the way back to img0 and verify Shift+Tab from pull0 lands on img0.
	si = focusPolicyRow(t, si, 0)
	si, _ = si.Update(shiftTab)
	if !si.InputFocused() || si.overlay.FocusedInput() != 0 {
		t.Fatalf("Shift+Tab from policy row 0 should focus image input 0, got kind=%d input=%d", si.focusKind, si.overlay.FocusedInput())
	}

	// Shift+Tab from the first image input (img0) wraps to the buttons.
	si, _ = si.Update(shiftTab)
	if si.focusKind != focusButtons {
		t.Fatalf("Shift+Tab from image input 0 should wrap to buttons, got kind=%d", si.focusKind)
	}
}

// TestSetImageOverlayDownAdvancesLikeTab verifies KeyDown advances focus
// identically to Tab through the full order.
func TestSetImageOverlayDownAdvancesLikeTab(t *testing.T) {
	siTab := NewSetImageOverlay(80, 30)
	siTab.Open("my-deploy", "default", testGVR, "deployments", testContainers)
	siDown := NewSetImageOverlay(80, 30)
	siDown.Open("my-deploy", "default", testGVR, "deployments", testContainers)

	// Drive both with their respective keys and compare focus state each step.
	for step := 0; step < 6; step++ {
		siTab, _ = siTab.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		siDown, _ = siDown.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		if siTab.focusKind != siDown.focusKind {
			t.Fatalf("step %d: Down focusKind=%d != Tab focusKind=%d", step, siDown.focusKind, siTab.focusKind)
		}
		if siTab.focusRow != siDown.focusRow {
			t.Fatalf("step %d: Down focusRow=%d != Tab focusRow=%d", step, siDown.focusRow, siTab.focusRow)
		}
		if siTab.overlay.FocusedInput() != siDown.overlay.FocusedInput() {
			t.Fatalf("step %d: Down focusedInput=%d != Tab focusedInput=%d", step, siDown.overlay.FocusedInput(), siTab.overlay.FocusedInput())
		}
	}
}

// TestSetImageOverlaySubmitImageChangedPolicyRevertedToDefault covers a
// container whose original PullPolicy is "Always": the user changes the image
// AND cycles the policy back to (default). The emitted change must carry the
// new image and an empty PullPolicy (policy revert suppressed, image still
// goes through).
func TestSetImageOverlaySubmitImageChangedPolicyRevertedToDefault(t *testing.T) {
	containers := []msgs.ContainerImageChange{
		{Name: "nginx", Image: "nginx:1.25", PullPolicy: "Always"},
	}
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", containers)

	// Change the image.
	si.overlay.Input(0).SetValue("nginx:1.26")

	// Cycle the policy from "Always" back to (default).
	si = focusPolicyRow(t, si, 0)
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyLeft}) // Always -> (default)
	if got := si.rows[0].policy.Value(); got != "" {
		t.Fatalf("expected policy reverted to default, got %q", got)
	}

	si, cmd := si.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command: the image change must still go through")
	}
	msg := cmd().(msgs.SetImageRequestedMsg)
	if len(msg.Images) != 1 {
		t.Fatalf("expected 1 change, got %d", len(msg.Images))
	}
	got := msg.Images[0]
	if got.Image != "nginx:1.26" {
		t.Errorf("expected Image 'nginx:1.26', got %q", got.Image)
	}
	if got.PullPolicy != "" {
		t.Errorf("expected empty PullPolicy (revert suppressed), got %q", got.PullPolicy)
	}
}
