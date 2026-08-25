package handlers

import (
	"errors"
	"net/http"

	"github.com/fvmoraes/ginger/pkg/response"
	"github.com/fvmoraes/kubepeep/internal/api"
	actionservice "github.com/fvmoraes/kubepeep/internal/services/actions"
)

const actionBodyLimit int64 = 64 << 10

type ActionHandlers struct {
	actions      actionservice.ActionService
	portForwards actionservice.PortForwardService
	exec         actionservice.ExecService
	selection    SelectionReader
}

func NewActionHandlers(actions actionservice.ActionService, portForwards actionservice.PortForwardService, exec actionservice.ExecService, selection SelectionReader) *ActionHandlers {
	return &ActionHandlers{actions: actions, portForwards: portForwards, exec: exec, selection: selection}
}

func (handler *ActionHandlers) Restart(w http.ResponseWriter, r *http.Request) {
	var request actionservice.RestartRequest
	if err := api.DecodeStrict(w, r, &request, actionBodyLimit); err != nil {
		api.WriteError(w, r, err)
		return
	}
	binding, _ := handler.selection.Snapshot()
	result, replayed, err := handler.actions.Restart(r.Context(), binding, workloadRouteTarget(r), r.Header.Get("Idempotency-Key"), request)
	if err != nil {
		api.WriteError(w, r, actionHTTPError(err))
		return
	}
	noStore(w)
	if replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = writeEnvelope(w, response.Envelope[actionservice.ActionAcceptedDTO]{Data: result})
}

func (handler *ActionHandlers) Scale(w http.ResponseWriter, r *http.Request) {
	var request actionservice.ScaleRequest
	if err := api.DecodeStrict(w, r, &request, actionBodyLimit); err != nil {
		api.WriteError(w, r, err)
		return
	}
	binding, _ := handler.selection.Snapshot()
	result, err := handler.actions.Scale(r.Context(), binding, workloadRouteTarget(r), request)
	if err != nil {
		api.WriteError(w, r, actionHTTPError(err))
		return
	}
	noStore(w)
	response.OK(w, result)
}

func (handler *ActionHandlers) DeletePod(w http.ResponseWriter, r *http.Request) {
	var request actionservice.PodDeleteRequest
	if err := api.DecodeStrict(w, r, &request, actionBodyLimit); err != nil {
		api.WriteError(w, r, err)
		return
	}
	binding, _ := handler.selection.Snapshot()
	result, err := handler.actions.DeletePod(r.Context(), binding, podRouteTarget(r), request)
	if err != nil {
		api.WriteError(w, r, actionHTTPError(err))
		return
	}
	noStore(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = writeEnvelope(w, response.Envelope[actionservice.ActionAcceptedDTO]{Data: result})
}

func (handler *ActionHandlers) CreatePortForward(w http.ResponseWriter, r *http.Request) {
	var request actionservice.PortForwardCreateRequest
	if err := api.DecodeStrict(w, r, &request, actionBodyLimit); err != nil {
		api.WriteError(w, r, err)
		return
	}
	binding, _ := handler.selection.Snapshot()
	result, replayed, err := handler.portForwards.Create(r.Context(), binding, podRouteTarget(r), r.Header.Get("Idempotency-Key"), request)
	if err != nil {
		api.WriteError(w, r, actionHTTPError(err))
		return
	}
	noStore(w)
	if replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	response.Created(w, result)
}

func (handler *ActionHandlers) ListPortForwards(w http.ResponseWriter, r *http.Request) {
	binding, _ := handler.selection.Snapshot()
	result, err := handler.portForwards.List(binding)
	if err != nil {
		api.WriteError(w, r, actionHTTPError(err))
		return
	}
	noStore(w)
	response.OK(w, result)
}

func (handler *ActionHandlers) ClosePortForward(w http.ResponseWriter, r *http.Request) {
	var request actionservice.PortForwardDeleteRequest
	if err := api.DecodeStrict(w, r, &request, actionBodyLimit); err != nil {
		api.WriteError(w, r, err)
		return
	}
	binding, _ := handler.selection.Snapshot()
	if err := handler.portForwards.Close(r.Context(), binding, r.PathValue("id"), request); err != nil {
		api.WriteError(w, r, actionHTTPError(err))
		return
	}
	noStore(w)
	response.NoContent(w)
}

func (handler *ActionHandlers) CreateExecTicket(w http.ResponseWriter, r *http.Request) {
	var request actionservice.ExecInit
	if err := api.DecodeStrict(w, r, &request, actionBodyLimit); err != nil {
		api.WriteError(w, r, err)
		return
	}
	binding, _ := handler.selection.Snapshot()
	result, err := handler.exec.CreateTicket(r.Context(), binding, podRouteTarget(r), request)
	if err != nil {
		api.WriteError(w, r, actionHTTPError(err))
		return
	}
	noStore(w)
	response.Created(w, result)
}

func workloadRouteTarget(r *http.Request) actionservice.RouteTarget {
	return actionservice.RouteTarget{
		Kind:      r.PathValue("kind"),
		Namespace: r.PathValue("namespace"),
		Name:      r.PathValue("name"),
	}
}

func podRouteTarget(r *http.Request) actionservice.RouteTarget {
	return actionservice.RouteTarget{
		Kind:      "pods",
		Namespace: r.PathValue("namespace"),
		Name:      r.PathValue("name"),
	}
}

type actionErrorDetails struct {
	Retryable bool                           `json:"retryable"`
	Fields    []actionservice.FieldViolation `json:"fields,omitempty"`
}

func actionHTTPError(err error) error {
	var actionError *actionservice.Error
	if !errors.As(err, &actionError) || actionError == nil {
		return api.NewHTTPError(http.StatusInternalServerError, api.CodeInternal, "Internal server error.", nil, err)
	}
	status := actionError.HTTPStatus
	if status == 0 {
		status = http.StatusRequestTimeout
	}
	details := actionErrorDetails{
		Retryable: actionError.Retryable,
		Fields:    append([]actionservice.FieldViolation(nil), actionError.Details...),
	}
	return api.NewHTTPError(status, string(actionError.Code), actionError.Message, details, err)
}
