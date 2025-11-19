package subscription

import (
	"fmt"
)

// StateMachine handles subscription state transitions
type StateMachine struct {
	// validTransitions defines which state transitions are allowed
	validTransitions map[SubscriptionStatus][]SubscriptionStatus
}

// NewStateMachine creates a new subscription state machine
func NewStateMachine() *StateMachine {
	return &StateMachine{
		validTransitions: map[SubscriptionStatus][]SubscriptionStatus{
			StatusPending: {
				StatusActive,
				StatusTrial,
				StatusIncomplete,
				StatusCanceled,
			},
			StatusActive: {
				StatusPastDue,
				StatusCanceled,
				StatusExpired,
			},
			StatusTrial: {
				StatusActive,
				StatusCanceled,
				StatusExpired,
			},
			StatusPastDue: {
				StatusActive,    // Payment recovered
				StatusCanceled,  // User cancels
				StatusExpired,   // Grace period ends
			},
			StatusIncomplete: {
				StatusActive,    // Payment completed
				StatusCanceled,  // User cancels
			},
			StatusCanceled: {
				StatusExpired,   // Period ends
				StatusActive,    // Reactivated
			},
			StatusExpired: {
				StatusActive,    // Re-subscribed
			},
		},
	}
}

// CanTransition checks if a transition from one status to another is valid
func (sm *StateMachine) CanTransition(from, to SubscriptionStatus) bool {
	allowedTransitions, ok := sm.validTransitions[from]
	if !ok {
		return false
	}

	for _, allowed := range allowedTransitions {
		if allowed == to {
			return true
		}
	}

	return false
}

// ValidateTransition returns an error if the transition is invalid
func (sm *StateMachine) ValidateTransition(from, to SubscriptionStatus) error {
	if !from.IsValid() {
		return fmt.Errorf("invalid source status: %s", from)
	}

	if !to.IsValid() {
		return fmt.Errorf("invalid target status: %s", to)
	}

	if !sm.CanTransition(from, to) {
		return fmt.Errorf("invalid state transition from %s to %s", from, to)
	}

	return nil
}

// GetAllowedTransitions returns all valid transitions from a given status
func (sm *StateMachine) GetAllowedTransitions(from SubscriptionStatus) []SubscriptionStatus {
	transitions, ok := sm.validTransitions[from]
	if !ok {
		return []SubscriptionStatus{}
	}
	return transitions
}

// TransitionReason represents the reason for a state transition
type TransitionReason struct {
	Code        string
	Description string
}

// Common transition reasons
var (
	ReasonUserCanceled = TransitionReason{
		Code:        "user_canceled",
		Description: "User canceled the subscription",
	}
	ReasonPaymentFailed = TransitionReason{
		Code:        "payment_failed",
		Description: "Payment method failed",
	}
	ReasonPaymentSucceeded = TransitionReason{
		Code:        "payment_succeeded",
		Description: "Payment was successful",
	}
	ReasonTrialEnded = TransitionReason{
		Code:        "trial_ended",
		Description: "Trial period ended",
	}
	ReasonPeriodEnded = TransitionReason{
		Code:        "period_ended",
		Description: "Subscription period ended",
	}
	ReasonUserUpgraded = TransitionReason{
		Code:        "user_upgraded",
		Description: "User upgraded to a higher plan",
	}
	ReasonUserDowngraded = TransitionReason{
		Code:        "user_downgraded",
		Description: "User downgraded to a lower plan",
	}
	ReasonUserReactivated = TransitionReason{
		Code:        "user_reactivated",
		Description: "User reactivated a canceled subscription",
	}
	ReasonGracePeriodExpired = TransitionReason{
		Code:        "grace_period_expired",
		Description: "Grace period for payment retry expired",
	}
	ReasonInitialPaymentIncomplete = TransitionReason{
		Code:        "initial_payment_incomplete",
		Description: "Initial payment requires action",
	}
	ReasonInitialPaymentComplete = TransitionReason{
		Code:        "initial_payment_complete",
		Description: "Initial payment completed",
	}
)

// StateTransition represents a state transition with context
type StateTransition struct {
	From   SubscriptionStatus
	To     SubscriptionStatus
	Reason TransitionReason
}

// NewStateTransition creates a new state transition
func NewStateTransition(from, to SubscriptionStatus, reason TransitionReason) StateTransition {
	return StateTransition{
		From:   from,
		To:     to,
		Reason: reason,
	}
}

// Validate checks if the state transition is valid
func (st StateTransition) Validate(sm *StateMachine) error {
	return sm.ValidateTransition(st.From, st.To)
}

// IsUpgrade checks if the transition represents an upgrade
func (st StateTransition) IsUpgrade() bool {
	return st.Reason.Code == ReasonUserUpgraded.Code
}

// IsDowngrade checks if the transition represents a downgrade
func (st StateTransition) IsDowngrade() bool {
	return st.Reason.Code == ReasonUserDowngraded.Code
}

// IsCancellation checks if the transition represents a cancellation
func (st StateTransition) IsCancellation() bool {
	return st.To == StatusCanceled
}

// IsReactivation checks if the transition represents a reactivation
func (st StateTransition) IsReactivation() bool {
	return st.From == StatusCanceled && st.To == StatusActive
}

// RequiresNotification checks if the transition should trigger a notification
func (st StateTransition) RequiresNotification() bool {
	switch st.To {
	case StatusCanceled, StatusExpired, StatusPastDue:
		return true
	default:
		return false
	}
}

// GetEventType returns the event type for logging this transition
func (st StateTransition) GetEventType() string {
	switch {
	case st.IsUpgrade():
		return EventTypeUpgraded
	case st.IsDowngrade():
		return EventTypeDowngraded
	case st.IsCancellation():
		return EventTypeCanceled
	case st.IsReactivation():
		return EventTypeReactivated
	case st.To == StatusExpired:
		return EventTypeExpired
	case st.To == StatusPastDue:
		return EventTypePastDue
	case st.To == StatusActive && st.From == StatusTrial:
		return EventTypeTrialEnded
	case st.To == StatusActive && st.From == StatusPending:
		return EventTypeActivated
	case st.To == StatusTrial:
		return EventTypeTrialStarted
	default:
		return fmt.Sprintf("status_changed.%s_to_%s", st.From, st.To)
	}
}
