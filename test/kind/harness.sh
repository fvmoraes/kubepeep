#!/bin/sh
set -eu

umask 077

harness_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cluster_name=${KUBEPEEP_KIND_CLUSTER:-kubepeep-f4}
cluster_context=kind-$cluster_name
kind_node_image=${KUBEPEEP_KIND_NODE_IMAGE:-kindest/node:v1.35.0@sha256:4613778f3cfcd10e615029370f5786704559103cf27bef934597ba562b269661}
request_timeout=${KUBEPEEP_KIND_TIMEOUT:-15s}
fixture_file=$harness_dir/rbac.yaml
cluster_file=$harness_dir/cluster.yaml
app_e2e_driver=$harness_dir/app_e2e.py
state_dir=${KUBEPEEP_KIND_STATE_DIR:-$harness_dir/.state}
managed_value=kubepeep-kind-harness
manual_subject=system:serviceaccount:kp-harness:manual-viewer
lister_subject=system:serviceaccount:kp-harness:namespace-lister
dashboard_subject=system:serviceaccount:kp-harness:dashboard-viewer
resource_subject=system:serviceaccount:kp-harness:resource-viewer
restart_subject=system:serviceaccount:kp-harness:restart-actor
scale_subject=system:serviceaccount:kp-harness:scale-actor
delete_subject=system:serviceaccount:kp-harness:delete-actor
portforward_subject=system:serviceaccount:kp-harness:portforward-actor
exec_subject=system:serviceaccount:kp-harness:exec-actor
app_e2e_subject=system:serviceaccount:kp-harness:app-e2e
refresh_binding=kp-refresh-probe

say() {
	printf '%s\n' "kind-harness: $*"
}

fail() {
	printf '%s\n' "kind-harness: $*" >&2
	exit 1
}

usage() {
	cat <<'EOF'
usage: test/kind/harness.sh COMMAND [ARG]

Commands:
  static                parse manifests locally and check forbidden grants
  create                create/reuse the dedicated Kind cluster and apply fixtures
  validate              validate F4-F7 reads, actions, denials and revocation
  kubeconfigs [DIR]     write short-lived restricted kubeconfigs (default: .state/)
  app-e2e BINARY        run optional black-box API checks with the real kubePeep binary
  refresh-grant         grant manual-viewer read access to kp-denied temporarily
  refresh-revoke        revoke only the harness-owned temporary grant

The script never deletes a Kind cluster or namespace. Set KUBEPEEP_KIND_CLUSTER
to use another dedicated cluster name.
EOF
}

validate_cluster_name() {
	case "$cluster_name" in
	''|*[!a-z0-9-]*|-*|*-)
		fail "KUBEPEEP_KIND_CLUSTER must be a lowercase DNS label"
		;;
	esac
	[ "${#cluster_name}" -le 63 ] ||
		fail "KUBEPEEP_KIND_CLUSTER must have at most 63 characters"
}

validate_kind_node_image() {
	case "$kind_node_image" in
	*:*@sha256:*) ;;
	*) fail "KUBEPEEP_KIND_NODE_IMAGE must preserve tag readability and pin sha256" ;;
	esac
	image_digest=${kind_node_image##*@sha256:}
	[ "${#image_digest}" -eq 64 ] || fail "KUBEPEEP_KIND_NODE_IMAGE sha256 must contain 64 hex characters"
	case "$image_digest" in
	*[!0-9a-f]*) fail "KUBEPEEP_KIND_NODE_IMAGE sha256 must be lowercase hexadecimal" ;;
	esac
	image_name=${kind_node_image%@sha256:*}
	case "$image_name" in
	*@*) fail "KUBEPEEP_KIND_NODE_IMAGE must contain exactly one digest" ;;
	esac
}

need() {
	command -v "$1" >/dev/null 2>&1 || fail "required command is unavailable: $1"
}

kube() {
	kubectl --context "$cluster_context" --request-timeout="$request_timeout" "$@"
}

kind_cluster_exists() {
	for existing_cluster in $(kind get clusters 2>/dev/null || true); do
		if [ "$existing_cluster" = "$cluster_name" ]; then
			return 0
		fi
	done
	return 1
}

guard_cluster() {
	need kubectl
	kube get --raw=/readyz >/dev/null 2>&1 ||
		fail "context $cluster_context is unavailable or not ready"
}

namespace_is_managed_or_absent() {
	namespace=$1
	if ! kube get namespace "$namespace" >/dev/null 2>&1; then
		return 0
	fi
	owner=$(kube get namespace "$namespace" \
		-o 'jsonpath={.metadata.labels.app\.kubernetes\.io/managed-by}' 2>/dev/null || true)
	[ "$owner" = "$managed_value" ] ||
		fail "namespace $namespace already exists and is not owned by this harness"
}

cluster_resource_is_managed_or_absent() {
	resource=$1
	name=$2
	if ! kube get "$resource" "$name" >/dev/null 2>&1; then
		return 0
	fi
	owner=$(kube get "$resource" "$name" \
		-o 'jsonpath={.metadata.labels.app\.kubernetes\.io/managed-by}' 2>/dev/null || true)
	[ "$owner" = "$managed_value" ] ||
		fail "$resource/$name already exists and is not owned by this harness"
}

namespaced_resource_is_managed_or_absent() {
	resource=$1
	namespace=$2
	name=$3
	if ! kube get "$resource" "$name" --namespace="$namespace" >/dev/null 2>&1; then
		return 0
	fi
	owner=$(kube get "$resource" "$name" --namespace="$namespace" \
		-o 'jsonpath={.metadata.labels.app\.kubernetes\.io/managed-by}' 2>/dev/null || true)
	[ "$owner" = "$managed_value" ] ||
		fail "$resource $namespace/$name already exists and is not owned by this harness"
}

apply_fixtures() {
	for fixture_namespace in kp-harness kp-allowed kp-denied; do
		namespace_is_managed_or_absent "$fixture_namespace"
	done
	cluster_resource_is_managed_or_absent clusterrole kp-namespace-lister
	cluster_resource_is_managed_or_absent clusterrolebinding kp-namespace-lister
	refresh_binding_is_managed_or_absent
	namespaced_resource_is_managed_or_absent serviceaccount kp-harness manual-viewer
	namespaced_resource_is_managed_or_absent serviceaccount kp-harness namespace-lister
	namespaced_resource_is_managed_or_absent serviceaccount kp-harness dashboard-viewer
	namespaced_resource_is_managed_or_absent serviceaccount kp-harness resource-viewer
	namespaced_resource_is_managed_or_absent serviceaccount kp-harness restart-actor
	namespaced_resource_is_managed_or_absent serviceaccount kp-harness scale-actor
	namespaced_resource_is_managed_or_absent serviceaccount kp-harness delete-actor
	namespaced_resource_is_managed_or_absent serviceaccount kp-harness portforward-actor
	namespaced_resource_is_managed_or_absent serviceaccount kp-harness exec-actor
	namespaced_resource_is_managed_or_absent serviceaccount kp-harness app-e2e
	namespaced_resource_is_managed_or_absent role kp-allowed kp-resource-reader
	namespaced_resource_is_managed_or_absent role kp-denied kp-resource-reader
	namespaced_resource_is_managed_or_absent rolebinding kp-allowed kp-resource-readers
	namespaced_resource_is_managed_or_absent role kp-allowed kp-dashboard-log-reader
	namespaced_resource_is_managed_or_absent rolebinding kp-allowed kp-dashboard-log-reader
	for action_resource in \
		"role kp-allowed kp-f6-resource-reader" \
		"rolebinding kp-allowed kp-f6-resource-reader" \
		"role kp-allowed kp-restart-action" \
		"rolebinding kp-allowed kp-restart-action" \
		"role kp-allowed kp-scale-action" \
		"rolebinding kp-allowed kp-scale-action" \
		"role kp-allowed kp-delete-action" \
		"rolebinding kp-allowed kp-delete-action" \
		"role kp-allowed kp-portforward-action" \
		"rolebinding kp-allowed kp-portforward-action" \
		"role kp-allowed kp-exec-action" \
		"rolebinding kp-allowed kp-exec-action"; do
		set -- $action_resource
		namespaced_resource_is_managed_or_absent "$1" "$2" "$3"
	done
	namespaced_resource_is_managed_or_absent configmap kp-allowed kp-fixture
	namespaced_resource_is_managed_or_absent configmap kp-denied kp-fixture
	namespaced_resource_is_managed_or_absent pod kp-allowed kp-fixture
	namespaced_resource_is_managed_or_absent pod kp-denied kp-fixture
	namespaced_resource_is_managed_or_absent deployment kp-allowed kp-degraded
	namespaced_resource_is_managed_or_absent pod kp-allowed kp-restarting
	namespaced_resource_is_managed_or_absent event kp-allowed kp-warning
	for fixture_resource in \
		"configmap kp-allowed kp-config" \
		"secret kp-allowed kp-secret-metadata" \
		"deployment kp-allowed kp-action-deployment" \
		"statefulset kp-allowed kp-action-statefulset" \
		"daemonset kp-allowed kp-daemonset" \
		"job kp-allowed kp-job" \
		"cronjob kp-allowed kp-cronjob" \
		"pod kp-allowed kp-interactive" \
		"pod kp-allowed kp-delete-probe" \
		"service kp-allowed kp-headless" \
		"service kp-allowed kp-service" \
		"ingress kp-allowed kp-ingress" \
		"endpointslice kp-allowed kp-service-v1" \
		"deployment kp-denied kp-action-deployment" \
		"statefulset kp-denied kp-action-statefulset" \
		"pod kp-denied kp-interactive" \
		"pod kp-denied kp-delete-probe"; do
		set -- $fixture_resource
		namespaced_resource_is_managed_or_absent "$1" "$2" "$3"
	done
	kube apply --server-side --field-manager=kubepeep-kind-harness -f "$fixture_file"
	hydrate_secret_fixture
}

hydrate_secret_fixture() (
	need python3
	secret_patch=$(mktemp)
	trap 'rm -f -- "${secret_patch:-}"' 0 HUP INT TERM
	python3 - "$secret_patch" <<'PY'
import base64
import json
import os
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
path.write_text(json.dumps({"data": {"opaque": base64.b64encode(os.urandom(32)).decode("ascii")}}), encoding="utf-8")
os.chmod(path, 0o600)
PY
	kube patch secret kp-secret-metadata --namespace=kp-allowed \
		--type=merge --patch-file="$secret_patch" >/dev/null
	rm -f -- "$secret_patch"
	secret_patch=
	trap - 0 HUP INT TERM
)

can_i() {
	subject=$1
	verb=$2
	requested_resource=$3
	namespace=$4
	resource_name=${5:-}
	case "$requested_resource" in
	*/*)
		resource=${requested_resource%%/*}
		subresource=${requested_resource#*/}
		resource_target=$resource
		if [ -n "$resource_name" ]; then
			resource_target=$resource/$resource_name
		fi
		if [ "$namespace" = "-" ]; then
			kube auth can-i "$verb" "$resource_target" --subresource="$subresource" \
				--as="$subject" >/dev/null 2>&1
		else
			kube auth can-i "$verb" "$resource_target" --subresource="$subresource" \
				--namespace="$namespace" --as="$subject" >/dev/null 2>&1
		fi
		;;
	*)
		resource=$requested_resource
		resource_target=$resource
		if [ -n "$resource_name" ]; then
			resource_target=$resource/$resource_name
		fi
		if [ "$namespace" = "-" ]; then
			kube auth can-i "$verb" "$resource_target" --as="$subject" >/dev/null 2>&1
		else
			kube auth can-i "$verb" "$resource_target" --namespace="$namespace" \
				--as="$subject" >/dev/null 2>&1
		fi
		;;
	esac
}

expect_can_i() {
	expected=$1
	subject=$2
	verb=$3
	requested_resource=$4
	namespace=$5
	description=$6
	resource_name=${7:-}

	if can_i "$subject" "$verb" "$requested_resource" "$namespace" "$resource_name"; then
		actual=yes
	else
		actual=no
	fi

	[ "$actual" = "$expected" ] ||
		fail "$description: expected $expected, got $actual ($verb $requested_resource, namespace=$namespace)"
	say "ok: $description ($actual)"
}

wait_can_i() {
	expected=$1
	subject=$2
	verb=$3
	requested_resource=$4
	namespace=$5
	description=$6
	resource_name=${7:-}
	attempt=0
	while [ "$attempt" -lt 30 ]; do
		if can_i "$subject" "$verb" "$requested_resource" "$namespace" "$resource_name"; then
			actual=yes
		else
			actual=no
		fi
		if [ "$actual" = "$expected" ]; then
			say "ok: $description ($actual)"
			return 0
		fi
		attempt=$((attempt + 1))
		sleep 1
	done
	fail "$description did not converge to $expected"
}

refresh_binding_is_managed_or_absent() {
	if ! kube get rolebinding "$refresh_binding" --namespace=kp-denied >/dev/null 2>&1; then
		return 0
	fi
	owner=$(kube get rolebinding "$refresh_binding" --namespace=kp-denied \
		-o 'jsonpath={.metadata.labels.app\.kubernetes\.io/managed-by}' 2>/dev/null || true)
	[ "$owner" = "$managed_value" ] ||
		fail "rolebinding kp-denied/$refresh_binding exists and is not owned by this harness"
}

grant_refresh_access() {
	refresh_binding_is_managed_or_absent
	kube apply --server-side --field-manager=kubepeep-kind-harness -f - >/dev/null <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: $refresh_binding
  namespace: kp-denied
  labels:
    app.kubernetes.io/managed-by: $managed_value
    kubepeep.dev/purpose: permission-refresh
subjects:
  - kind: ServiceAccount
    name: manual-viewer
    namespace: kp-harness
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: kp-resource-reader
EOF
}

revoke_refresh_access() {
	refresh_binding_is_managed_or_absent
	kube delete rolebinding "$refresh_binding" --namespace=kp-denied --ignore-not-found >/dev/null
}

validate_baseline() {
	expect_can_i yes "$manual_subject" list pods kp-allowed "single scope reads the allowed namespace"
	expect_can_i yes "$manual_subject" get configmaps kp-allowed "manual viewer reads allowed fixtures"
	expect_can_i no "$manual_subject" list pods kp-denied "list scope reports the denied namespace"
	expect_can_i no "$manual_subject" list namespaces - "manual viewer cannot activate all"
	expect_can_i no "$manual_subject" get pods/log kp-allowed "pod logs remain denied"
	expect_can_i no "$manual_subject" create pods/exec kp-allowed "pod exec remains denied"
	expect_can_i no "$manual_subject" create pods/portforward kp-allowed "pod port-forward remains denied"
	expect_can_i no "$manual_subject" list secrets kp-allowed "secrets remain denied"
	expect_can_i no "$manual_subject" delete pods kp-allowed "mutations remain denied"
	expect_can_i no "$manual_subject" '*' '*' - "manual viewer is not cluster-admin"

	expect_can_i yes "$lister_subject" list namespaces - "namespace lister can activate all"
	expect_can_i yes "$lister_subject" list pods kp-allowed "all scope can read an authorized namespace"
	expect_can_i no "$lister_subject" list pods kp-denied "all scope preserves per-namespace denial"
	expect_can_i no "$lister_subject" list secrets kp-allowed "all identity cannot read secrets"
	expect_can_i no "$lister_subject" '*' '*' - "namespace lister is not cluster-admin"

	expect_can_i yes "$dashboard_subject" list pods kp-allowed "dashboard viewer reads pods"
	expect_can_i yes "$dashboard_subject" get pods/log kp-allowed "dashboard viewer reads bounded pod logs"
	expect_can_i yes "$dashboard_subject" list events kp-allowed "dashboard viewer reads Warning events"
	expect_can_i yes "$dashboard_subject" list deployments kp-allowed "dashboard viewer reads degraded workloads"
	expect_can_i no "$dashboard_subject" list pods kp-denied "dashboard preserves partial namespace denial"
	expect_can_i no "$dashboard_subject" list namespaces - "dashboard viewer cannot broaden its scope"

	kube get pod kp-fixture --namespace=kp-allowed --as="$manual_subject" -o name >/dev/null
	if kube get pod kp-fixture --namespace=kp-denied --as="$manual_subject" -o name >/dev/null 2>&1; then
		fail "manual viewer unexpectedly read the denied fixture"
	fi
	kube get namespace kp-allowed --as="$lister_subject" -o name >/dev/null
	kube get deployment kp-degraded --namespace=kp-allowed --as="$dashboard_subject" -o name >/dev/null
	kube get events --namespace=kp-allowed --as="$dashboard_subject" -o name | \
		grep -Fx 'event/kp-warning' >/dev/null || fail "dashboard viewer could not list the Warning fixture"
	if kube get --raw=/apis/metrics.k8s.io/v1beta1 >/dev/null 2>&1; then
		fail "the canonical dashboard fixture requires Metrics API to be absent"
	fi
	say "ok: Metrics API is absent for the optional-feature scenario"
	attempt=0
	while [ "$attempt" -lt 60 ]; do
		restarts=$(kube get pod kp-restarting --namespace=kp-allowed --as="$dashboard_subject" \
			-o 'jsonpath={.status.containerStatuses[0].restartCount}' 2>/dev/null || true)
		case "$restarts" in
		''|*[!0-9]*) ;;
		*)
			if [ "$restarts" -gt 0 ]; then
				break
			fi
			;;
		esac
		attempt=$((attempt + 1))
		sleep 2
	done
	[ "${restarts:-0}" -gt 0 ] || fail "restart fixture did not restart within the bounded wait"
	kube logs kp-restarting --namespace=kp-allowed --container=synthetic-log \
		--previous --as="$dashboard_subject" 2>/dev/null | grep -F 'synthetic dashboard fixture unavailable' >/dev/null ||
		fail "dashboard viewer could not read the synthetic previous log"
	say "ok: restart, degraded workload, Warning event and bounded log fixtures are observable"
	say "ok: single/list/all scenarios use real API resources"
}

validate_refresh() {
	refresh_binding_is_managed_or_absent
	# Recover safely from an interrupted previous harness run before asserting
	# the baseline. Unknown bindings are rejected by the ownership guard.
	revoke_refresh_access
	wait_can_i no "$manual_subject" list pods kp-denied "refresh baseline is denied"

	refresh_active=yes
	trap 'if [ "${refresh_active:-}" = yes ]; then revoke_refresh_access || true; fi' 0 HUP INT TERM
	grant_refresh_access
	wait_can_i yes "$manual_subject" list pods kp-denied "RBAC grant becomes authoritative"
	revoke_refresh_access
	refresh_active=no
	wait_can_i no "$manual_subject" list pods kp-denied "RBAC revoke becomes authoritative"
	trap - 0 HUP INT TERM
}

validate_resource_authorization() {
	for requested_resource in pods services configmaps deployments.apps statefulsets.apps daemonsets.apps jobs.batch cronjobs.batch ingresses.networking.k8s.io endpointslices.discovery.k8s.io; do
		expect_can_i yes "$resource_subject" get "$requested_resource" kp-allowed "F6 can get $requested_resource"
		expect_can_i yes "$resource_subject" list "$requested_resource" kp-allowed "F6 can list $requested_resource"
		expect_can_i yes "$resource_subject" watch "$requested_resource" kp-allowed "F6 can watch $requested_resource"
		expect_can_i no "$resource_subject" get "$requested_resource" kp-denied "F6 detail is denied for $requested_resource outside scope"
		expect_can_i no "$resource_subject" list "$requested_resource" kp-denied "F6 list is denied for $requested_resource outside scope"
		expect_can_i no "$resource_subject" watch "$requested_resource" kp-denied "F6 watch is denied for $requested_resource outside scope"
	done
	expect_can_i yes "$resource_subject" list events kp-allowed "F6 can list events"
	expect_can_i yes "$resource_subject" watch events kp-allowed "F6 can watch events"
	expect_can_i no "$resource_subject" get events kp-allowed "events have no detail capability"
	expect_can_i no "$resource_subject" list events kp-denied "F6 event list is denied outside scope"
	expect_can_i no "$resource_subject" watch events kp-denied "F6 event watch is denied outside scope"
	expect_can_i yes "$resource_subject" get pods/log kp-allowed "F6 can read current and previous logs" kp-interactive
	expect_can_i yes "$resource_subject" get secrets kp-allowed "F6 can request one Secret metadata response" kp-secret-metadata
	expect_can_i yes "$resource_subject" list secrets kp-allowed "F6 can list Secret metadata"
	expect_can_i no "$resource_subject" watch secrets kp-allowed "Secrets are excluded from shared watches"
	expect_can_i no "$resource_subject" get secrets kp-denied "Secret metadata remains denied outside scope" kp-secret-metadata
	expect_can_i no "$resource_subject" create pods/exec kp-allowed "read identity cannot exec" kp-interactive
	expect_can_i no "$resource_subject" create pods/portforward kp-allowed "read identity cannot port-forward" kp-interactive
	expect_can_i no "$resource_subject" delete pods kp-allowed "read identity cannot delete" kp-delete-probe
}

validate_action_authorization() {
	expect_can_i yes "$restart_subject" patch deployments.apps kp-allowed "restart identity can patch only the target Deployment" kp-action-deployment
	expect_can_i no "$restart_subject" update deployments.apps/scale kp-allowed "restart identity cannot scale" kp-action-deployment
	expect_can_i no "$restart_subject" delete pods kp-allowed "restart identity cannot delete Pods" kp-delete-probe
	expect_can_i no "$restart_subject" patch deployments.apps kp-allowed "restart identity is resourceName-bound" kp-degraded

	expect_can_i yes "$scale_subject" update deployments.apps/scale kp-allowed "scale identity can update Deployment scale" kp-action-deployment
	expect_can_i yes "$scale_subject" update statefulsets.apps/scale kp-allowed "scale identity can update StatefulSet scale" kp-action-statefulset
	expect_can_i no "$scale_subject" patch deployments.apps kp-allowed "scale identity cannot restart" kp-action-deployment
	expect_can_i no "$scale_subject" update deployments.apps/scale kp-allowed "scale identity is resourceName-bound" kp-degraded

	expect_can_i yes "$delete_subject" delete pods kp-allowed "delete identity can delete the target Pod" kp-delete-probe
	expect_can_i no "$delete_subject" create pods/exec kp-allowed "delete identity cannot exec" kp-interactive

	expect_can_i yes "$portforward_subject" create pods/portforward kp-allowed "port-forward identity can create the exact subresource" kp-interactive
	expect_can_i no "$portforward_subject" create pods/exec kp-allowed "port-forward identity cannot exec" kp-interactive

	expect_can_i yes "$exec_subject" create pods/exec kp-allowed "exec identity can create the exact subresource" kp-interactive
	expect_can_i no "$exec_subject" create pods/portforward kp-allowed "exec identity cannot port-forward" kp-interactive

	for action_check in \
		"$restart_subject patch deployments.apps kp-action-deployment" \
		"$scale_subject update deployments.apps/scale kp-action-deployment" \
		"$delete_subject delete pods kp-delete-probe" \
		"$portforward_subject create pods/portforward kp-interactive" \
		"$exec_subject create pods/exec kp-interactive"; do
		set -- $action_check
		expect_can_i no "$1" "$2" "$3" kp-denied "action $2 $3 is denied in kp-denied" "$4"
	done

	expect_can_i yes "$app_e2e_subject" patch deployments.apps kp-allowed "black-box identity can restart" kp-action-deployment
	expect_can_i yes "$app_e2e_subject" update deployments.apps/scale kp-allowed "black-box identity can scale" kp-action-deployment
	expect_can_i yes "$app_e2e_subject" delete pods kp-allowed "black-box identity can delete" kp-delete-probe
	expect_can_i yes "$app_e2e_subject" create pods/portforward kp-allowed "black-box identity can port-forward" kp-interactive
	expect_can_i yes "$app_e2e_subject" create pods/exec kp-allowed "black-box identity can exec" kp-interactive
	expect_can_i no "$app_e2e_subject" patch deployments.apps kp-denied "black-box identity is denied in kp-denied" kp-action-deployment
}

validate_resource_operations() {
	for list_resource in \
		pods services configmaps deployments.apps statefulsets.apps daemonsets.apps \
		jobs.batch cronjobs.batch ingresses.networking.k8s.io endpointslices.discovery.k8s.io; do
		kube get "$list_resource" --namespace=kp-allowed --as="$resource_subject" -o name >/dev/null
	done
	kube get events --namespace=kp-allowed --as="$resource_subject" -o name | \
		grep -Fx 'event/kp-warning' >/dev/null || fail "F6 event LIST omitted the Warning fixture"
	for fixture_resource in \
		"deployment kp-action-deployment" \
		"statefulset kp-action-statefulset" \
		"daemonset kp-daemonset" \
		"job kp-job" \
		"cronjob kp-cronjob" \
		"pod kp-interactive" \
		"service kp-service" \
		"ingress kp-ingress" \
		"endpointslice kp-service-v1" \
		"configmap kp-config"; do
		set -- $fixture_resource
		kube get "$1" "$2" --namespace=kp-allowed --as="$resource_subject" -o name >/dev/null
	done
	for yaml_resource in \
		"deployment kp-action-deployment" \
		"statefulset kp-action-statefulset" \
		"daemonset kp-daemonset" \
		"job kp-job" \
		"cronjob kp-cronjob" \
		"pod kp-interactive" \
		"service kp-service" \
		"ingress kp-ingress" \
		"endpointslice kp-service-v1" \
		"configmap kp-config"; do
		set -- $yaml_resource
		kube get "$1" "$2" --namespace=kp-allowed --as="$resource_subject" -o yaml >/dev/null
	done
	config_value=$(kube get configmap kp-config --namespace=kp-allowed --as="$resource_subject" \
		-o 'jsonpath={.data.mode}')
	[ "$config_value" = canonical ] || fail "ConfigMap detail did not return the canonical data"
	unset config_value

	kube wait --for=condition=Ready pod/kp-interactive --namespace=kp-allowed --timeout=120s >/dev/null
	kube logs kp-interactive --namespace=kp-allowed --container=web --as="$resource_subject" 2>/dev/null |
		grep -F 'synthetic current log ready' >/dev/null || fail "F6 current log fixture is not readable"
	kube logs kp-restarting --namespace=kp-allowed --container=synthetic-log --previous \
		--as="$resource_subject" 2>/dev/null | grep -F 'synthetic dashboard fixture unavailable' >/dev/null ||
		fail "F6 previous log fixture is not readable"

	watch_output=$(mktemp)
	watch_pid=
	watch_label_set=no
	watch_cleanup() {
		if [ -n "${watch_pid:-}" ] && kill -0 "$watch_pid" >/dev/null 2>&1; then
			kill "$watch_pid" >/dev/null 2>&1 || true
			wait "$watch_pid" >/dev/null 2>&1 || true
		fi
		if [ "${watch_label_set:-no}" = yes ]; then
			kube label configmap kp-config --namespace=kp-allowed kubepeep.dev/watch-probe- >/dev/null 2>&1 || true
		fi
		rm -f -- "${watch_output:-}"
	}
	trap watch_cleanup 0 HUP INT TERM
	kube get configmap kp-config --namespace=kp-allowed --watch-only -o name \
		--as="$resource_subject" >"$watch_output" 2>/dev/null &
	watch_pid=$!
	sleep 1
	watch_probe=probe-$$-$(date +%s)
	kube label configmap kp-config --namespace=kp-allowed \
		kubepeep.dev/watch-probe="$watch_probe" --overwrite >/dev/null
	watch_label_set=yes
	attempt=0
	while [ "$attempt" -lt 40 ] && [ ! -s "$watch_output" ]; do
		if ! kill -0 "$watch_pid" >/dev/null 2>&1; then
			break
		fi
		attempt=$((attempt + 1))
		sleep 1
	done
	[ -s "$watch_output" ] || fail "authorized watch did not observe the ConfigMap update"
	kube label configmap kp-config --namespace=kp-allowed kubepeep.dev/watch-probe- >/dev/null
	watch_label_set=no
	watch_cleanup
	watch_pid=
	watch_output=
	trap - 0 HUP INT TERM

	if kube get pods --namespace=kp-denied --watch-only --as="$resource_subject" >/dev/null 2>&1; then
		fail "denied watch unexpectedly started"
	fi
	if kube get pods --namespace=kp-denied --as="$resource_subject" -o name >/dev/null 2>&1; then
		fail "denied list unexpectedly succeeded"
	fi
	if kube get deployment kp-action-deployment --namespace=kp-denied \
		--as="$resource_subject" -o name >/dev/null 2>&1; then
		fail "denied detail unexpectedly succeeded"
	fi
	say "ok: F6 list/detail/YAML, current/previous logs and allowed/denied watch use the real API"
	say "ok: the Secret fixture carries runtime-only data; this harness never reads or prints it"
}

extract_fixture_object() (
	object_kind=$1
	object_namespace=$2
	object_name=$3
	object_file=$4
	need awk
	awk -v expected_kind="$object_kind" -v expected_namespace="$object_namespace" -v expected_name="$object_name" '
function reset_document() {
    document = ""
    document_kind = ""
    document_namespace = ""
    document_name = ""
    in_metadata = 0
}
function finish_document() {
    if (document_kind == expected_kind && document_namespace == expected_namespace && document_name == expected_name) {
        printf "%s", document
        matches++
    }
}
BEGIN { reset_document() }
/^---[[:space:]]*$/ {
    finish_document()
    reset_document()
    next
}
{
    document = document $0 ORS
    if ($0 ~ /^kind:[[:space:]]*/) {
        value = $0
        sub(/^kind:[[:space:]]*/, "", value)
        document_kind = value
    }
    if ($0 == "metadata:") {
        in_metadata = 1
        next
    }
    if (in_metadata && $0 ~ /^[^[:space:]]/) {
        in_metadata = 0
    }
    if (in_metadata && document_name == "" && $0 ~ /^  name:[[:space:]]*/) {
        value = $0
        sub(/^  name:[[:space:]]*/, "", value)
        document_name = value
    }
    if (in_metadata && document_namespace == "" && $0 ~ /^  namespace:[[:space:]]*/) {
        value = $0
        sub(/^  namespace:[[:space:]]*/, "", value)
        document_namespace = value
    }
}
END {
    finish_document()
    if (matches != 1) {
        exit 42
    }
}
' "$fixture_file" >"$object_file" || fail "canonical fixture object was not found exactly once"
)

reapply_fixture_object() (
	object_kind=$1
	object_namespace=$2
	object_name=$3
	object_file=$(mktemp)
	trap 'rm -f -- "${object_file:-}"' 0 HUP INT TERM
	extract_fixture_object "$object_kind" "$object_namespace" "$object_name" "$object_file"
	kube apply --server-side --field-manager=kubepeep-kind-harness -f "$object_file" >/dev/null
	rm -f -- "$object_file"
	object_file=
	trap - 0 HUP INT TERM
)

restore_fixture_pod() (
	object_namespace=$1
	object_name=$2
	if kube get pod "$object_name" --namespace="$object_namespace" >/dev/null 2>&1; then
		deletion_timestamp=$(kube get pod "$object_name" --namespace="$object_namespace" \
			-o 'jsonpath={.metadata.deletionTimestamp}' 2>/dev/null || true)
		if [ -n "$deletion_timestamp" ]; then
			attempt=0
			while kube get pod "$object_name" --namespace="$object_namespace" >/dev/null 2>&1; do
				[ "$attempt" -lt 60 ] || return 1
				attempt=$((attempt + 1))
				sleep 1
			done
		fi
	fi
	namespaced_resource_is_managed_or_absent pod "$object_namespace" "$object_name"
	reapply_fixture_object Pod "$object_namespace" "$object_name"
)

wait_for_pod_absent() (
	object_namespace=$1
	object_name=$2
	attempt=0
	while kube get pod "$object_name" --namespace="$object_namespace" >/dev/null 2>&1; do
		[ "$attempt" -lt 60 ] || return 1
		attempt=$((attempt + 1))
		sleep 1
	done
)

rolebinding_is_managed() {
	binding_name=$1
	owner=$(kube get rolebinding "$binding_name" --namespace=kp-allowed \
		-o 'jsonpath={.metadata.labels.app\.kubernetes\.io/managed-by}' 2>/dev/null || true)
	[ "$owner" = "$managed_value" ] || fail "rolebinding kp-allowed/$binding_name is absent or not harness-owned"
}

revoke_action_binding() {
	binding_name=$1
	rolebinding_is_managed "$binding_name"
	kube delete rolebinding "$binding_name" --namespace=kp-allowed >/dev/null
}

restore_action_binding() {
	binding_name=$1
	namespaced_resource_is_managed_or_absent rolebinding kp-allowed "$binding_name"
	reapply_fixture_object RoleBinding kp-allowed "$binding_name"
}

scale_subresource() (
	scale_namespace=$1
	scale_kind=$2
	scale_name=$3
	scale_replicas=$4
	scale_as=$5
	case "$scale_kind" in
	deployment)
		scale_plural=deployments
		;;
	statefulset)
		scale_plural=statefulsets
		;;
	*)
		fail "unsupported scale fixture kind: $scale_kind"
		;;
	esac
	scale_path=/apis/apps/v1/namespaces/$scale_namespace/$scale_plural/$scale_name/scale
	scale_rv=$(kube get "$scale_kind" "$scale_name" --namespace="$scale_namespace" \
		-o 'jsonpath={.metadata.resourceVersion}')
	scale_body=$(mktemp)
	trap 'rm -f -- "${scale_body:-}"' 0 HUP INT TERM
	printf '%s\n' "{\"apiVersion\":\"autoscaling/v1\",\"kind\":\"Scale\",\"metadata\":{\"name\":\"$scale_name\",\"namespace\":\"$scale_namespace\",\"resourceVersion\":\"$scale_rv\"},\"spec\":{\"replicas\":$scale_replicas}}" >"$scale_body"
	if [ "$scale_as" = - ]; then
		kube replace --raw="$scale_path" -f "$scale_body" >/dev/null
	else
		kube replace --raw="$scale_path" -f "$scale_body" --as="$scale_as" >/dev/null
	fi
	scale_status=$?
	rm -f -- "$scale_body"
	scale_body=
	trap - 0 HUP INT TERM
	exit "$scale_status"
)

validate_restart_and_scale_operations() {
	mutation_cleanup_needed=yes
	mutation_cleanup() {
		if [ "${mutation_cleanup_needed:-no}" != yes ]; then
			return
		fi
		kube patch deployment kp-action-deployment --namespace=kp-allowed --type=merge \
			--patch='{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":null}}}}}' \
			>/dev/null 2>&1 || true
		kube patch deployment kp-action-deployment --namespace=kp-denied --type=merge \
			--patch='{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":null}}}}}' \
			>/dev/null 2>&1 || true
		scale_subresource kp-allowed deployment kp-action-deployment 1 - >/dev/null 2>&1 || true
		scale_subresource kp-allowed statefulset kp-action-statefulset 1 - >/dev/null 2>&1 || true
		scale_subresource kp-denied deployment kp-action-deployment 1 - >/dev/null 2>&1 || true
		scale_subresource kp-denied statefulset kp-action-statefulset 1 - >/dev/null 2>&1 || true
	}
	trap mutation_cleanup 0 HUP INT TERM
	kube patch deployment kp-action-deployment --namespace=kp-allowed --type=merge \
		--patch='{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":null}}}}}' >/dev/null
	kube patch deployment kp-action-deployment --namespace=kp-allowed --type=merge \
		--patch='{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"2026-08-17T00:00:00Z"}}}}}' \
		--as="$restart_subject" >/dev/null
	restart_value=$(kube get deployment kp-action-deployment --namespace=kp-allowed \
		-o 'jsonpath={.spec.template.metadata.annotations.kubectl\.kubernetes\.io/restartedAt}')
	[ "$restart_value" = 2026-08-17T00:00:00Z ] || fail "authorized restart patch was not persisted"
	kube patch deployment kp-action-deployment --namespace=kp-allowed --type=merge \
		--patch='{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":null}}}}}' >/dev/null
	if kube patch deployment kp-action-deployment --namespace=kp-denied --type=merge \
		--patch='{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"2026-08-17T00:00:00Z"}}}}}' \
		--as="$restart_subject" >/dev/null 2>&1; then
		fail "restart unexpectedly succeeded in kp-denied"
	fi

	scale_subresource kp-allowed deployment kp-action-deployment 2 "$scale_subject"
	scale_subresource kp-allowed deployment kp-action-deployment 1 -
	scale_subresource kp-allowed statefulset kp-action-statefulset 2 "$scale_subject"
	scale_subresource kp-allowed statefulset kp-action-statefulset 1 -
	if scale_subresource kp-denied deployment kp-action-deployment 2 "$scale_subject" >/dev/null 2>&1; then
		fail "Deployment scale unexpectedly succeeded in kp-denied"
	fi
	if scale_subresource kp-denied statefulset kp-action-statefulset 2 "$scale_subject" >/dev/null 2>&1; then
		fail "StatefulSet scale unexpectedly succeeded in kp-denied"
	fi
	mutation_cleanup
	mutation_cleanup_needed=no
	trap - 0 HUP INT TERM
	say "ok: restart and both scale subresources mutate real fixtures and restore their state"
}

validate_delete_operation() {
	delete_restore_needed=yes
	delete_cleanup() {
		if [ "${delete_restore_needed:-no}" = yes ]; then
			restore_fixture_pod kp-allowed kp-delete-probe >/dev/null 2>&1 || true
			restore_fixture_pod kp-denied kp-delete-probe >/dev/null 2>&1 || true
		fi
	}
	trap delete_cleanup 0 HUP INT TERM
	kube delete pod kp-delete-probe --namespace=kp-allowed --as="$delete_subject" --wait=false >/dev/null
	wait_for_pod_absent kp-allowed kp-delete-probe || fail "authorized Pod deletion did not complete"
	restore_fixture_pod kp-allowed kp-delete-probe || fail "deleted Pod fixture could not be restored"
	if kube delete pod kp-delete-probe --namespace=kp-denied --as="$delete_subject" \
		--wait=false >/dev/null 2>&1; then
		fail "Pod deletion unexpectedly succeeded in kp-denied"
	fi
	kube get pod kp-delete-probe --namespace=kp-denied -o name >/dev/null
	delete_restore_needed=no
	trap - 0 HUP INT TERM
	say "ok: Pod delete is authorized only in kp-allowed and the fixture is restored"
}

bounded_denied_portforward() (
	denied_namespace=$1
	denied_log=$(mktemp)
	denied_timeout=$(mktemp)
	denied_pid=
	denied_timer_pid=
	denied_cleanup() {
		if [ -n "${denied_pid:-}" ] && kill -0 "$denied_pid" >/dev/null 2>&1; then
			kill "$denied_pid" >/dev/null 2>&1 || true
		fi
		if [ -n "${denied_timer_pid:-}" ] && kill -0 "$denied_timer_pid" >/dev/null 2>&1; then
			kill "$denied_timer_pid" >/dev/null 2>&1 || true
		fi
		wait "${denied_pid:-0}" >/dev/null 2>&1 || true
		wait "${denied_timer_pid:-0}" >/dev/null 2>&1 || true
		rm -f -- "$denied_log" "$denied_timeout"
	}
	trap denied_cleanup 0 HUP INT TERM
	kube port-forward pod/kp-interactive :8080 --address=127.0.0.1 --namespace="$denied_namespace" \
		--as="$portforward_subject" >"$denied_log" 2>&1 &
	denied_pid=$!
	(
		sleep 10
		: >"$denied_timeout"
		kill "$denied_pid" >/dev/null 2>&1 || true
	) &
	denied_timer_pid=$!
	if wait "$denied_pid"; then
		denied_status=0
	else
		denied_status=$?
	fi
	denied_pid=
	if kill -0 "$denied_timer_pid" >/dev/null 2>&1; then
		kill "$denied_timer_pid" >/dev/null 2>&1 || true
	fi
	wait "$denied_timer_pid" >/dev/null 2>&1 || true
	denied_timer_pid=
	[ ! -s "$denied_timeout" ] || fail "denied port-forward was not rejected before the bounded deadline"
	[ "$denied_status" -ne 0 ] || fail "denied port-forward exited successfully"
	rm -f -- "$denied_log" "$denied_timeout"
	denied_log=
	denied_timeout=
	trap - 0 HUP INT TERM
)

probe_portforward_http() {
	python3 - "$1" <<'PY'
import sys
import urllib.request

opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
with opener.open(f"http://127.0.0.1:{int(sys.argv[1])}/", timeout=5) as response:
    if response.status != 200 or response.read(3) != b"ok\n":
        raise SystemExit("unexpected response through port-forward")
PY
}

validate_portforward_operation() {
	kube wait --for=condition=Ready pod/kp-interactive --namespace=kp-allowed --timeout=120s >/dev/null
	bounded_denied_portforward kp-denied
	portforward_log=$(mktemp)
	portforward_pid=
	portforward_binding_revoked=no
	portforward_cleanup() {
		if [ -n "${portforward_pid:-}" ] && kill -0 "$portforward_pid" >/dev/null 2>&1; then
			kill "$portforward_pid" >/dev/null 2>&1 || true
			wait "$portforward_pid" >/dev/null 2>&1 || true
		fi
		if [ "${portforward_binding_revoked:-no}" = yes ]; then
			restore_action_binding kp-portforward-action >/dev/null 2>&1 || true
		fi
		rm -f -- "${portforward_log:-}"
	}
	trap portforward_cleanup 0 HUP INT TERM
	kube port-forward pod/kp-interactive :8080 --address=127.0.0.1 --namespace=kp-allowed \
		--as="$portforward_subject" >"$portforward_log" 2>&1 &
	portforward_pid=$!
	attempt=0
	local_port=
	while [ "$attempt" -lt 60 ]; do
		local_port=$(sed -n 's/^Forwarding from 127\.0\.0\.1:\([0-9][0-9]*\) -> 8080$/\1/p' "$portforward_log" | head -n 1)
		if [ -n "$local_port" ]; then
			break
		fi
		kill -0 "$portforward_pid" >/dev/null 2>&1 || fail "authorized port-forward exited during setup"
		attempt=$((attempt + 1))
		sleep 1
	done
	case "$local_port" in
	''|*[!0-9]*) fail "authorized port-forward did not expose a loopback port" ;;
	esac
	probe_portforward_http "$local_port"
	revoke_action_binding kp-portforward-action
	portforward_binding_revoked=yes
	wait_can_i no "$portforward_subject" create pods/portforward kp-allowed "new port-forward is denied after revocation" kp-interactive
	probe_portforward_http "$local_port"
	bounded_denied_portforward kp-allowed
	portforward_cleanup
	portforward_pid=
	portforward_log=
	wait_can_i yes "$portforward_subject" create pods/portforward kp-allowed "port-forward grant is restored" kp-interactive
	portforward_binding_revoked=no
	trap - 0 HUP INT TERM
	say "ok: real loopback port-forward works, a post-revocation upgrade is denied, and cleanup closes it"
}

validate_exec_operation() {
	kube exec kp-interactive --namespace=kp-allowed --container=utility --as="$exec_subject" \
		-- /bin/true >/dev/null 2>&1 || fail "authorized exec did not complete"
	if kube exec kp-interactive --namespace=kp-denied --container=utility --as="$exec_subject" \
		-- /bin/true >/dev/null 2>&1; then
		fail "exec unexpectedly succeeded in kp-denied"
	fi
	revoke_action_binding kp-exec-action
	exec_binding_revoked=yes
	exec_revocation_cleanup() {
		if [ "${exec_binding_revoked:-no}" = yes ]; then
			restore_action_binding kp-exec-action >/dev/null 2>&1 || true
		fi
	}
	trap exec_revocation_cleanup 0 HUP INT TERM
	wait_can_i no "$exec_subject" create pods/exec kp-allowed "exec is denied after ticket-time revocation" kp-interactive
	if kube exec kp-interactive --namespace=kp-allowed --container=utility --as="$exec_subject" \
		-- /bin/true >/dev/null 2>&1; then
		fail "exec unexpectedly upgraded after revocation"
	fi
	exec_revocation_cleanup
	wait_can_i yes "$exec_subject" create pods/exec kp-allowed "exec grant is restored" kp-interactive
	exec_binding_revoked=no
	trap - 0 HUP INT TERM
	say "ok: exec uses the real subresource and rechecks denial after revocation"
}

validate_phase_six_and_seven() {
	validate_resource_authorization
	validate_action_authorization
	validate_resource_operations
	validate_restart_and_scale_operations
	validate_delete_operation
	validate_portforward_operation
	validate_exec_operation
}

decode_base64() {
	if base64 --decode >/dev/null 2>&1 <<EOF
$1
EOF
	then
		printf '%s' "$1" | base64 --decode
	elif base64 -D >/dev/null 2>&1 <<EOF
$1
EOF
	then
		printf '%s' "$1" | base64 -D
	else
		fail "base64 decoder does not support --decode or -D"
	fi
}

write_kubeconfig() {
	service_account=$1
	context_name=$2
	output_file=$3
	server=$4
	ca_file=$5
	build_file=$6
	token=$(kube create token "$service_account" --namespace=kp-harness --duration=1h)
	[ -n "$token" ] || fail "TokenRequest returned an empty token for $service_account"
	kubectl config set-cluster "$cluster_name" --server="$server" \
		--certificate-authority="$ca_file" --embed-certs=true \
		--kubeconfig="$build_file" >/dev/null
	kubectl config set-credentials "$service_account" --token="$token" \
		--kubeconfig="$build_file" >/dev/null
	kubectl config set-context "$context_name" --cluster="$cluster_name" \
		--user="$service_account" --namespace=kp-allowed \
		--kubeconfig="$build_file" >/dev/null
	kubectl config use-context "$context_name" --kubeconfig="$build_file" >/dev/null
	chmod 600 "$build_file"
	mv -- "$build_file" "$output_file"
	unset token
}

kubeconfig_target_is_safe() {
	output_file=$1
	expected_context=$2
	if [ ! -e "$output_file" ]; then
		return 0
	fi
	[ ! -L "$output_file" ] && [ -f "$output_file" ] ||
		fail "refusing to replace non-regular kubeconfig target: $output_file"
	current_context=$(kubectl config current-context --kubeconfig="$output_file" 2>/dev/null || true)
	[ "$current_context" = "$expected_context" ] ||
		fail "refusing to replace kubeconfig not generated for context $expected_context: $output_file"
}

export_kubeconfigs() (
	output_dir=${1:-$state_dir}
	mkdir -p -- "$output_dir"
	manual_output=$output_dir/manual-viewer.kubeconfig
	lister_output=$output_dir/namespace-lister.kubeconfig
	dashboard_output=$output_dir/dashboard-viewer.kubeconfig
	resource_output=$output_dir/resource-viewer.kubeconfig
	restart_output=$output_dir/restart-actor.kubeconfig
	scale_output=$output_dir/scale-actor.kubeconfig
	delete_output=$output_dir/delete-actor.kubeconfig
	portforward_output=$output_dir/portforward-actor.kubeconfig
	exec_output=$output_dir/exec-actor.kubeconfig
	app_e2e_output=$output_dir/app-e2e.kubeconfig
	kubeconfig_target_is_safe "$manual_output" kubepeep-f4-manual
	kubeconfig_target_is_safe "$lister_output" kubepeep-f4-all
	kubeconfig_target_is_safe "$dashboard_output" kubepeep-f5-dashboard
	kubeconfig_target_is_safe "$resource_output" kubepeep-f6-resources
	kubeconfig_target_is_safe "$restart_output" kubepeep-f7-restart
	kubeconfig_target_is_safe "$scale_output" kubepeep-f7-scale
	kubeconfig_target_is_safe "$delete_output" kubepeep-f7-delete
	kubeconfig_target_is_safe "$portforward_output" kubepeep-f7-portforward
	kubeconfig_target_is_safe "$exec_output" kubepeep-f7-exec
	kubeconfig_target_is_safe "$app_e2e_output" kubepeep-f7-app-e2e
	temporary_dir=$(mktemp -d)
	trap 'rm -rf -- "${temporary_dir:-}"' 0 HUP INT TERM
	server=$(kubectl config view --raw --minify --flatten --context="$cluster_context" \
		-o 'jsonpath={.clusters[0].cluster.server}')
	ca_data=$(kubectl config view --raw --minify --flatten --context="$cluster_context" \
		-o 'jsonpath={.clusters[0].cluster.certificate-authority-data}')
	[ -n "$server" ] || fail "the Kind context does not expose an API server"
	[ -n "$ca_data" ] || fail "the Kind context does not expose embedded CA data"
	ca_file=$temporary_dir/ca.crt
	decode_base64 "$ca_data" >"$ca_file"
	write_kubeconfig manual-viewer kubepeep-f4-manual \
		"$manual_output" "$server" "$ca_file" "$temporary_dir/manual-viewer.kubeconfig"
	write_kubeconfig namespace-lister kubepeep-f4-all \
		"$lister_output" "$server" "$ca_file" "$temporary_dir/namespace-lister.kubeconfig"
	write_kubeconfig dashboard-viewer kubepeep-f5-dashboard \
		"$dashboard_output" "$server" "$ca_file" "$temporary_dir/dashboard-viewer.kubeconfig"
	write_kubeconfig resource-viewer kubepeep-f6-resources \
		"$resource_output" "$server" "$ca_file" "$temporary_dir/resource-viewer.kubeconfig"
	write_kubeconfig restart-actor kubepeep-f7-restart \
		"$restart_output" "$server" "$ca_file" "$temporary_dir/restart-actor.kubeconfig"
	write_kubeconfig scale-actor kubepeep-f7-scale \
		"$scale_output" "$server" "$ca_file" "$temporary_dir/scale-actor.kubeconfig"
	write_kubeconfig delete-actor kubepeep-f7-delete \
		"$delete_output" "$server" "$ca_file" "$temporary_dir/delete-actor.kubeconfig"
	write_kubeconfig portforward-actor kubepeep-f7-portforward \
		"$portforward_output" "$server" "$ca_file" "$temporary_dir/portforward-actor.kubeconfig"
	write_kubeconfig exec-actor kubepeep-f7-exec \
		"$exec_output" "$server" "$ca_file" "$temporary_dir/exec-actor.kubeconfig"
	write_kubeconfig app-e2e kubepeep-f7-app-e2e \
		"$app_e2e_output" "$server" "$ca_file" "$temporary_dir/app-e2e.kubeconfig"
	rm -rf -- "$temporary_dir"
	trap - 0 HUP INT TERM
	say "restricted, one-hour kubeconfigs written to $output_dir"
)

app_command() {
	env \
		HOME="$app_home" \
		XDG_CONFIG_HOME="$app_config_home" \
		XDG_DATA_HOME="$app_data_home" \
		XDG_CACHE_HOME="$app_cache_home" \
		XDG_RUNTIME_DIR="$app_runtime_home" \
		KUBECONFIG="$app_kubeconfig" \
		"$@"
}

app_instance_cleanup() {
	if [ -n "${app_driver_pid:-}" ] && kill -0 "$app_driver_pid" >/dev/null 2>&1; then
		kill "$app_driver_pid" >/dev/null 2>&1 || true
		wait "$app_driver_pid" >/dev/null 2>&1 || true
	fi
	app_driver_pid=
	if [ -n "${app_pid:-}" ] && kill -0 "$app_pid" >/dev/null 2>&1; then
		app_command "$app_binary" stop >/dev/null 2>&1 || true
		attempt=0
		while [ "$attempt" -lt 30 ] && kill -0 "$app_pid" >/dev/null 2>&1; do
			attempt=$((attempt + 1))
			sleep 1
		done
		if kill -0 "$app_pid" >/dev/null 2>&1; then
			kill "$app_pid" >/dev/null 2>&1 || true
		fi
		wait "$app_pid" >/dev/null 2>&1 || true
	fi
	app_pid=
}

write_offline_kubeconfig() {
	offline_output=$1
	offline_build=$2
	kubectl config set-cluster kubepeep-offline --server=https://127.0.0.1:1 \
		--insecure-skip-tls-verify=true --kubeconfig="$offline_build" >/dev/null
	kubectl config set-credentials offline-synthetic \
		--token=offline-synthetic-not-a-cluster-credential --kubeconfig="$offline_build" >/dev/null
	kubectl config set-context kubepeep-f8-offline --cluster=kubepeep-offline \
		--user=offline-synthetic --namespace=kp-allowed --kubeconfig="$offline_build" >/dev/null
	kubectl config use-context kubepeep-f8-offline --kubeconfig="$offline_build" >/dev/null
	chmod 600 "$offline_build"
	mv -- "$offline_build" "$offline_output"
}

wait_app_driver_marker() {
	marker_name=$1
	maximum_attempts=$2
	attempt=0
	while [ "$attempt" -lt "$maximum_attempts" ]; do
		[ ! -f "$app_control_dir/$marker_name" ] || return 0
		kill -0 "$app_driver_pid" >/dev/null 2>&1 || fail "app-e2e driver exited before marker $marker_name"
		attempt=$((attempt + 1))
		sleep 1
	done
	fail "app-e2e driver did not publish marker $marker_name before the bounded deadline"
}

coordinate_allowed_driver() {
	wait_app_driver_marker f6-ready 240
	revoke_action_binding kp-f6-resource-reader
	app_f6_binding_revoked=yes
	touch "$app_control_dir/f6-revoked"
	wait_app_driver_marker f6-done 100
	restore_action_binding kp-f6-resource-reader
	wait_can_i yes "$app_e2e_subject" list pods kp-allowed "app F6 grant is restored"
	app_f6_binding_revoked=no
	touch "$app_control_dir/f6-restored"
	wait_app_driver_marker exec-ready 40
	revoke_action_binding kp-exec-action
	app_exec_binding_revoked=yes
	touch "$app_control_dir/exec-revoked"
	wait_app_driver_marker exec-done 40
	restore_action_binding kp-exec-action
	wait_can_i yes "$app_e2e_subject" create pods/exec kp-allowed "app exec grant is restored" kp-interactive
	app_exec_binding_revoked=no
}

validate_app_sensitive_absence() {
	python3 - "$app_instance_root" "$app_kubeconfig" "$cluster_context" <<'PY'
import base64
import binascii
import json
import pathlib
import subprocess
import sys

root = pathlib.Path(sys.argv[1])
kubeconfig = pathlib.Path(sys.argv[2])
context = sys.argv[3]

def kubectl_json(arguments):
    completed = subprocess.run(
        ["kubectl", *arguments],
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
        timeout=20,
    )
    return json.loads(completed.stdout)

configuration = kubectl_json(["config", "view", "--raw", f"--kubeconfig={kubeconfig}", "-o", "json"])
tokens = []
for user in configuration.get("users", []):
    token = user.get("user", {}).get("token")
    if isinstance(token, str) and token:
        tokens.append(token.encode("utf-8"))
secret = kubectl_json(
    ["--context", context, "get", "secret", "kp-secret-metadata", "--namespace=kp-allowed", "-o", "json"]
)
encoded_secret = secret.get("data", {}).get("opaque")
if not isinstance(encoded_secret, str) or not encoded_secret:
    raise SystemExit("runtime Secret fixture omitted its opaque payload")
try:
    decoded_secret = base64.b64decode(encoded_secret, validate=True)
except (binascii.Error, ValueError) as error:
    raise SystemExit("runtime Secret fixture payload is invalid") from error
needles = [
    encoded_secret.encode("ascii"),
    decoded_secret,
    *tokens,
    b"kp-ticket.",
    b"synthetic current log ready",
    b"synthetic dashboard fixture unavailable",
    b"kp-exec-stdout",
    b"kp-exec-stderr",
]
for path in root.rglob("*"):
    if path.is_symlink() or not path.is_file():
        continue
    if path.stat().st_size > 64 << 20:
        raise SystemExit("app state file exceeded the bounded sensitive-data scan")
    contents = path.read_bytes()
    if any(needle and needle in contents for needle in needles):
        raise SystemExit("app output/state contains a Kubernetes credential or Secret payload")
PY
}

run_app_instance() {
	app_mode=$1
	app_namespace=$2
	app_kubeconfig=$3
	app_context=$4
	shift 4
	app_instance_root=$app_temporary_root/$app_mode
	app_home=$app_instance_root/home
	app_config_home=$app_instance_root/config
	app_data_home=$app_instance_root/data
	app_cache_home=$app_instance_root/cache
	app_runtime_home=$app_instance_root/runtime
	app_log=$app_instance_root/process.log
	app_driver_log=$app_instance_root/driver.log
	app_control_dir=$app_instance_root/control
	mkdir -p -- "$app_home" "$app_config_home" "$app_data_home" "$app_cache_home" "$app_runtime_home" "$app_control_dir"
	chmod 700 "$app_home" "$app_config_home" "$app_data_home" "$app_cache_home" "$app_runtime_home" "$app_control_dir"
	app_command "$app_binary" start --no-browser --kubeconfig "$app_kubeconfig" \
		--context "$app_context" --namespace "$app_namespace" >"$app_log" 2>&1 &
	app_pid=$!
	attempt=0
	app_status=
	while [ "$attempt" -lt 60 ]; do
		if ! kill -0 "$app_pid" >/dev/null 2>&1; then
			fail "app-e2e $app_mode instance exited before reporting status"
		fi
		app_status=$(app_command "$app_binary" status 2>/dev/null || true)
		case "$app_status" in
		running\ pid=*\ port=*\ protocol=kubepeep-control/v1)
			break
			;;
		esac
		attempt=$((attempt + 1))
		sleep 1
	done
	app_port=$(printf '%s\n' "$app_status" | sed -n 's|^running pid=[0-9][0-9]* port=\([0-9][0-9]*\) protocol=kubepeep-control/v1$|\1|p')
	case "$app_port" in
	''|*[!0-9]*) fail "app-e2e could not discover the loopback port through kubePeep status" ;;
	esac
	if [ "$app_mode" = allowed ]; then
		python3 "$app_e2e_driver" --origin "http://127.0.0.1:$app_port" --mode "$app_mode" \
			--control-dir "$app_control_dir" --scan-root "$app_instance_root" "$@" >"$app_driver_log" 2>&1 &
	else
		python3 "$app_e2e_driver" --origin "http://127.0.0.1:$app_port" --mode "$app_mode" \
			"$@" >"$app_driver_log" 2>&1 &
	fi
	app_driver_pid=$!
	if [ "$app_mode" = allowed ]; then
		coordinate_allowed_driver
	fi
	if ! wait "$app_driver_pid"; then
		app_driver_pid=
		fail "app-e2e $app_mode product driver failed"
	fi
	app_driver_pid=
	app_instance_cleanup
	validate_app_sensitive_absence
}

run_app_e2e() {
	[ "$#" -eq 1 ] || { usage >&2; exit 2; }
	guard_cluster
	need python3
	app_binary=$1
	[ -f "$app_binary" ] && [ -x "$app_binary" ] || fail "app-e2e BINARY must be a regular executable"
	app_binary_dir=$(CDPATH= cd -- "$(dirname -- "$app_binary")" && pwd)
	app_binary=$app_binary_dir/$(basename -- "$app_binary")
	apply_fixtures
	revoke_refresh_access
	kube wait --for=condition=Ready pod/kp-interactive --namespace=kp-allowed --timeout=120s >/dev/null
	attempt=0
	restart_count=
	while [ "$attempt" -lt 60 ]; do
		restart_count=$(kube get pod kp-restarting --namespace=kp-allowed \
			-o 'jsonpath={.status.containerStatuses[0].restartCount}' 2>/dev/null || true)
		case "$restart_count" in
		''|*[!0-9]*) ;;
		*)
			[ "$restart_count" -gt 0 ] && break
			;;
		esac
		attempt=$((attempt + 1))
		sleep 2
	done
	[ "${restart_count:-0}" -gt 0 ] || fail "app-e2e previous-log fixture did not restart in time"
	app_temporary_root=$(mktemp -d)
	export_kubeconfigs "$app_temporary_root/kubeconfigs" >/dev/null
	app_e2e_kubeconfig=$app_temporary_root/kubeconfigs/app-e2e.kubeconfig
	app_lister_kubeconfig=$app_temporary_root/kubeconfigs/namespace-lister.kubeconfig
	app_offline_kubeconfig=$app_temporary_root/kubeconfigs/offline-e2e.kubeconfig
	write_offline_kubeconfig "$app_offline_kubeconfig" "$app_temporary_root/kubeconfigs/offline-e2e.build"
	app_kubeconfig=$app_e2e_kubeconfig
	app_pid=
	app_driver_pid=
	app_f6_binding_revoked=no
	app_exec_binding_revoked=no
	app_restore_state=yes
	app_e2e_cleanup() {
		app_instance_cleanup
		if [ "${app_f6_binding_revoked:-no}" = yes ]; then
			restore_action_binding kp-f6-resource-reader >/dev/null 2>&1 || true
			app_f6_binding_revoked=no
		fi
		if [ "${app_exec_binding_revoked:-no}" = yes ]; then
			restore_action_binding kp-exec-action >/dev/null 2>&1 || true
			app_exec_binding_revoked=no
		fi
		if [ "${app_restore_state:-no}" = yes ]; then
			restore_fixture_pod kp-allowed kp-delete-probe >/dev/null 2>&1 || true
			restore_fixture_pod kp-denied kp-delete-probe >/dev/null 2>&1 || true
			kube patch deployment kp-action-deployment --namespace=kp-allowed --type=merge \
				--patch='{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":null}}}}}' \
				>/dev/null 2>&1 || true
			kube patch deployment kp-action-deployment --namespace=kp-denied --type=merge \
				--patch='{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":null}}}}}' \
				>/dev/null 2>&1 || true
			scale_subresource kp-allowed deployment kp-action-deployment 1 - >/dev/null 2>&1 || true
			scale_subresource kp-denied deployment kp-action-deployment 1 - >/dev/null 2>&1 || true
		fi
		rm -rf -- "${app_temporary_root:-}"
	}
	trap app_e2e_cleanup 0 HUP INT TERM

	run_app_instance allowed kp-allowed "$app_e2e_kubeconfig" kubepeep-f7-app-e2e
	restore_fixture_pod kp-allowed kp-delete-probe || fail "black-box deleted Pod fixture could not be restored"
	kube patch deployment kp-action-deployment --namespace=kp-allowed --type=merge \
		--patch='{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":null}}}}}' >/dev/null
	scale_subresource kp-allowed deployment kp-action-deployment 1 -
	denied_deployment_rv=$(kube get deployment kp-action-deployment --namespace=kp-denied \
		-o 'jsonpath={.metadata.resourceVersion}')
	denied_pod_rv=$(kube get pod kp-delete-probe --namespace=kp-denied \
		-o 'jsonpath={.metadata.resourceVersion}')
	denied_pod_uid=$(kube get pod kp-delete-probe --namespace=kp-denied -o 'jsonpath={.metadata.uid}')
	run_app_instance denied kp-denied "$app_e2e_kubeconfig" kubepeep-f7-app-e2e \
		--deployment-rv "$denied_deployment_rv" --pod-rv "$denied_pod_rv" --pod-uid "$denied_pod_uid"
	run_app_instance all kp-allowed "$app_lister_kubeconfig" kubepeep-f4-all
	run_app_instance offline kp-allowed "$app_offline_kubeconfig" kubepeep-f8-offline

	app_e2e_cleanup
	app_temporary_root=
	trap - 0 HUP INT TERM
	say "real context/scope, dashboard, SSE/log, WebSocket exec, revocation and offline product paths passed"
}

static_validate() {
	need python3
	need awk
	if grep -Eq 'cluster-admin|resources:[[:space:]]*\[[^]]*"\*"|verbs:[[:space:]]*\[[^]]*"\*"' "$fixture_file"; then
		fail "fixture contains cluster-admin or wildcard RBAC"
	fi
	extraction_probe=$(mktemp)
	trap 'rm -f -- "${extraction_probe:-}"' 0 HUP INT TERM
	extract_fixture_object RoleBinding kp-allowed kp-exec-action "$extraction_probe"
	[ -s "$extraction_probe" ] || fail "RoleBinding extraction produced an empty manifest"
	extract_fixture_object Pod kp-allowed kp-delete-probe "$extraction_probe"
	[ -s "$extraction_probe" ] || fail "Pod extraction produced an empty manifest"
	rm -f -- "$extraction_probe"
	extraction_probe=
	trap - 0 HUP INT TERM
	python3 - "$fixture_file" "$cluster_file" "$app_e2e_driver" "$kind_node_image" <<'PY'
import pathlib
import re
import sys

try:
    import yaml
except ImportError as error:
    raise SystemExit("PyYAML is required for offline manifest validation") from error

fixture_path, cluster_path, app_e2e_path = map(pathlib.Path, sys.argv[1:4])
kind_node_image = sys.argv[4]
immutable_image = re.compile(r"^[^@\s]+:[^@\s]+@sha256:[0-9a-f]{64}$")
if immutable_image.fullmatch(kind_node_image) is None:
    raise SystemExit("Kind node image must retain a tag and be pinned by sha256 digest")
documents = list(yaml.safe_load_all(fixture_path.read_text(encoding="utf-8")))
if not documents or any(not isinstance(document, dict) for document in documents):
    raise SystemExit("fixture must contain only non-empty YAML mappings")
indexed = {}
for index, document in enumerate(documents, start=1):
    metadata = document.get("metadata")
    if not isinstance(document.get("apiVersion"), str):
        raise SystemExit(f"fixture document {index} has no apiVersion")
    if not isinstance(document.get("kind"), str):
        raise SystemExit(f"fixture document {index} has no kind")
    if not isinstance(metadata, dict) or not isinstance(metadata.get("name"), str):
        raise SystemExit(f"fixture document {index} has no metadata.name")
    labels = metadata.get("labels", {})
    if labels.get("app.kubernetes.io/managed-by") != "kubepeep-kind-harness":
        raise SystemExit(f"fixture document {index} has no harness ownership label")
    key = (document["kind"], metadata.get("namespace", ""), metadata["name"])
    if key in indexed:
        raise SystemExit(f"fixture contains duplicate object {key}")
    indexed[key] = document
    if document["kind"] in {"Role", "ClusterRole"}:
        for rule in document.get("rules", []):
            if any("*" in rule.get(field, []) for field in ("apiGroups", "resources", "verbs")):
                raise SystemExit(f"fixture document {index} contains wildcard RBAC")
            forbidden_verbs = {"impersonate", "bind", "escalate"}.intersection(rule.get("verbs", []))
            if forbidden_verbs:
                raise SystemExit(f"fixture document {index} contains privileged RBAC verbs: {sorted(forbidden_verbs)}")
    if document["kind"] in {"RoleBinding", "ClusterRoleBinding"}:
        if document.get("roleRef", {}).get("name") == "cluster-admin":
            raise SystemExit(f"fixture document {index} binds cluster-admin")
    pod_spec = document.get("spec", {})
    if document["kind"] in {"Deployment", "StatefulSet", "DaemonSet", "Job"}:
        pod_spec = pod_spec.get("template", {}).get("spec", {})
    elif document["kind"] == "CronJob":
        pod_spec = pod_spec.get("jobTemplate", {}).get("spec", {}).get("template", {}).get("spec", {})
    if document["kind"] == "Pod" or document["kind"] in {"Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob"}:
        for container_group in ("initContainers", "containers", "ephemeralContainers"):
            for container in pod_spec.get(container_group, []):
                image = container.get("image") if isinstance(container, dict) else None
                if not isinstance(image, str) or immutable_image.fullmatch(image) is None:
                    raise SystemExit(f"fixture document {index} contains a mutable container image")

required = {
    ("Namespace", "", "kp-harness"),
    ("Namespace", "", "kp-allowed"),
    ("Namespace", "", "kp-denied"),
    ("ServiceAccount", "kp-harness", "manual-viewer"),
    ("ServiceAccount", "kp-harness", "namespace-lister"),
    ("ServiceAccount", "kp-harness", "dashboard-viewer"),
    ("ServiceAccount", "kp-harness", "resource-viewer"),
    ("ServiceAccount", "kp-harness", "restart-actor"),
    ("ServiceAccount", "kp-harness", "scale-actor"),
    ("ServiceAccount", "kp-harness", "delete-actor"),
    ("ServiceAccount", "kp-harness", "portforward-actor"),
    ("ServiceAccount", "kp-harness", "exec-actor"),
    ("ServiceAccount", "kp-harness", "app-e2e"),
    ("Role", "kp-allowed", "kp-resource-reader"),
    ("Role", "kp-denied", "kp-resource-reader"),
    ("RoleBinding", "kp-allowed", "kp-resource-readers"),
    ("Role", "kp-allowed", "kp-dashboard-log-reader"),
    ("RoleBinding", "kp-allowed", "kp-dashboard-log-reader"),
    ("Role", "kp-allowed", "kp-f6-resource-reader"),
    ("RoleBinding", "kp-allowed", "kp-f6-resource-reader"),
    ("Role", "kp-allowed", "kp-restart-action"),
    ("RoleBinding", "kp-allowed", "kp-restart-action"),
    ("Role", "kp-allowed", "kp-scale-action"),
    ("RoleBinding", "kp-allowed", "kp-scale-action"),
    ("Role", "kp-allowed", "kp-delete-action"),
    ("RoleBinding", "kp-allowed", "kp-delete-action"),
    ("Role", "kp-allowed", "kp-portforward-action"),
    ("RoleBinding", "kp-allowed", "kp-portforward-action"),
    ("Role", "kp-allowed", "kp-exec-action"),
    ("RoleBinding", "kp-allowed", "kp-exec-action"),
    ("ClusterRole", "", "kp-namespace-lister"),
    ("ClusterRoleBinding", "", "kp-namespace-lister"),
    ("Pod", "kp-allowed", "kp-fixture"),
    ("Pod", "kp-denied", "kp-fixture"),
    ("Deployment", "kp-allowed", "kp-degraded"),
    ("Pod", "kp-allowed", "kp-restarting"),
    ("Event", "kp-allowed", "kp-warning"),
    ("Deployment", "kp-allowed", "kp-action-deployment"),
    ("StatefulSet", "kp-allowed", "kp-action-statefulset"),
    ("DaemonSet", "kp-allowed", "kp-daemonset"),
    ("Job", "kp-allowed", "kp-job"),
    ("CronJob", "kp-allowed", "kp-cronjob"),
    ("Pod", "kp-allowed", "kp-interactive"),
    ("Pod", "kp-allowed", "kp-delete-probe"),
    ("Service", "kp-allowed", "kp-service"),
    ("Service", "kp-allowed", "kp-headless"),
    ("Ingress", "kp-allowed", "kp-ingress"),
    ("EndpointSlice", "kp-allowed", "kp-service-v1"),
    ("ConfigMap", "kp-allowed", "kp-config"),
    ("Secret", "kp-allowed", "kp-secret-metadata"),
    ("Deployment", "kp-denied", "kp-action-deployment"),
    ("StatefulSet", "kp-denied", "kp-action-statefulset"),
    ("Pod", "kp-denied", "kp-interactive"),
    ("Pod", "kp-denied", "kp-delete-probe"),
}
missing = required.difference(indexed)
if missing:
    raise SystemExit(f"fixture is missing required objects: {sorted(missing)}")
if any(key[0] == "RoleBinding" and key[1] == "kp-denied" for key in indexed):
    raise SystemExit("denied namespace must not have a baseline RoleBinding")

lister_role = indexed[("ClusterRole", "", "kp-namespace-lister")]
expected_lister_rule = {"apiGroups": [""], "resources": ["namespaces"], "verbs": ["get", "list", "watch"]}
if lister_role.get("rules") != [expected_lister_rule]:
    raise SystemExit("namespace lister ClusterRole is broader than the canonical rule")
lister_binding = indexed[("ClusterRoleBinding", "", "kp-namespace-lister")]
if lister_binding.get("roleRef", {}).get("name") != "kp-namespace-lister":
    raise SystemExit("namespace lister binding references an unexpected ClusterRole")
if lister_binding.get("subjects") != [
    {"kind": "ServiceAccount", "name": "namespace-lister", "namespace": "kp-harness"}
]:
    raise SystemExit("namespace lister ClusterRoleBinding has unexpected subjects")
for document in documents:
    if document.get("kind") != "ClusterRoleBinding":
        continue
    for subject in document.get("subjects", []):
        if subject.get("name") in {"manual-viewer", "app-e2e"}:
            raise SystemExit("manual viewer and app-e2e must not have a ClusterRoleBinding")

dashboard_role = indexed[("Role", "kp-allowed", "kp-dashboard-log-reader")]
if dashboard_role.get("rules") != [{"apiGroups": [""], "resources": ["pods/log"], "verbs": ["get"]}]:
    raise SystemExit("dashboard log Role is broader than get pods/log")

expected_roles = {
    "kp-f6-resource-reader": [
        {"apiGroups": [""], "resources": ["pods", "services", "configmaps"], "verbs": ["get", "list", "watch"]},
        {"apiGroups": [""], "resources": ["secrets"], "verbs": ["get", "list"]},
        {"apiGroups": [""], "resources": ["pods/log"], "verbs": ["get"]},
        {"apiGroups": [""], "resources": ["events"], "verbs": ["list", "watch"]},
        {"apiGroups": ["apps"], "resources": ["deployments", "statefulsets", "daemonsets"], "verbs": ["get", "list", "watch"]},
        {"apiGroups": ["batch"], "resources": ["jobs", "cronjobs"], "verbs": ["get", "list", "watch"]},
        {"apiGroups": ["networking.k8s.io"], "resources": ["ingresses"], "verbs": ["get", "list", "watch"]},
        {"apiGroups": ["discovery.k8s.io"], "resources": ["endpointslices"], "verbs": ["get", "list", "watch"]},
    ],
    "kp-restart-action": [{"apiGroups": ["apps"], "resources": ["deployments"], "resourceNames": ["kp-action-deployment"], "verbs": ["patch"]}],
    "kp-scale-action": [
        {"apiGroups": ["apps"], "resources": ["deployments/scale"], "resourceNames": ["kp-action-deployment"], "verbs": ["update"]},
        {"apiGroups": ["apps"], "resources": ["statefulsets/scale"], "resourceNames": ["kp-action-statefulset"], "verbs": ["update"]},
    ],
    "kp-delete-action": [{"apiGroups": [""], "resources": ["pods"], "resourceNames": ["kp-delete-probe"], "verbs": ["delete"]}],
    "kp-portforward-action": [
        {"apiGroups": [""], "resources": ["pods"], "resourceNames": ["kp-interactive"], "verbs": ["get"]},
        {"apiGroups": [""], "resources": ["pods/portforward"], "resourceNames": ["kp-interactive"], "verbs": ["create"]},
    ],
    "kp-exec-action": [
        {"apiGroups": [""], "resources": ["pods"], "resourceNames": ["kp-interactive"], "verbs": ["get"]},
        {"apiGroups": [""], "resources": ["pods/exec"], "resourceNames": ["kp-interactive"], "verbs": ["create"]},
    ],
}
for role_name, expected_ruleset in expected_roles.items():
    role = indexed[("Role", "kp-allowed", role_name)]
    if role.get("rules") != expected_ruleset:
        raise SystemExit(f"{role_name} differs from its canonical narrow rules")

expected_binding_subjects = {
    "kp-f6-resource-reader": {"resource-viewer", "app-e2e"},
    "kp-restart-action": {"restart-actor", "app-e2e"},
    "kp-scale-action": {"scale-actor", "app-e2e"},
    "kp-delete-action": {"delete-actor", "app-e2e"},
    "kp-portforward-action": {"portforward-actor", "app-e2e"},
    "kp-exec-action": {"exec-actor", "app-e2e"},
}
for binding_name, expected_subjects in expected_binding_subjects.items():
    binding = indexed[("RoleBinding", "kp-allowed", binding_name)]
    subjects = binding.get("subjects", [])
    actual_subjects = {
        (subject.get("kind"), subject.get("name"), subject.get("namespace")) for subject in subjects
    }
    canonical_subjects = {("ServiceAccount", name, "kp-harness") for name in expected_subjects}
    if (
        actual_subjects != canonical_subjects
        or len(subjects) != len(canonical_subjects)
        or binding.get("roleRef", {}).get("name") != binding_name
    ):
        raise SystemExit(f"{binding_name} does not bind only its canonical subjects")

secret = indexed[("Secret", "kp-allowed", "kp-secret-metadata")]
if "data" in secret or "stringData" in secret or secret.get("type") != "Opaque":
    raise SystemExit("the versioned Secret fixture must contain metadata only")
warning = indexed[("Event", "kp-allowed", "kp-warning")]
if warning.get("type") != "Warning" or warning.get("count") != 3:
    raise SystemExit("dashboard Warning event fixture is not canonical")
degraded = indexed[("Deployment", "kp-allowed", "kp-degraded")]
degraded_pod_spec = degraded.get("spec", {}).get("template", {}).get("spec", {})
if degraded_pod_spec.get("nodeSelector") != {"kubepeep.dev/nonexistent-node": "true"}:
    raise SystemExit("dashboard degraded fixture must be unschedulable without a missing image")

cluster = yaml.safe_load(cluster_path.read_text(encoding="utf-8"))
if not isinstance(cluster, dict):
    raise SystemExit("Kind config is not a YAML mapping")
if cluster.get("apiVersion") != "kind.x-k8s.io/v1alpha4" or cluster.get("kind") != "Cluster":
    raise SystemExit("Kind config has an unexpected apiVersion/kind")
if not isinstance(cluster.get("nodes"), list) or not cluster["nodes"]:
    raise SystemExit("Kind config must declare at least one node")
compile(app_e2e_path.read_text(encoding="utf-8"), str(app_e2e_path), "exec")
PY
	say "manifests parse locally, pin every image digest and contain no cluster-admin/wildcard grants"
}

create_cluster() {
	need kind
	need docker
	need kubectl
	docker info >/dev/null 2>&1 || fail "Docker daemon is unavailable; no cluster state was changed"
	if kind_cluster_exists; then
		say "reusing dedicated cluster $cluster_name (it will not be deleted)"
	else
		kind create cluster --name "$cluster_name" --image "$kind_node_image" --config "$cluster_file" --wait 120s
	fi
	guard_cluster
	apply_fixtures
	revoke_refresh_access
	validate_baseline
	validate_resource_authorization
	validate_action_authorization
	say "cluster is ready at context $cluster_context"
}

validate_cluster_name
validate_kind_node_image
command=${1:-}
case "$command" in
static)
	[ "$#" -eq 1 ] || { usage >&2; exit 2; }
	static_validate
	;;
create)
	[ "$#" -eq 1 ] || { usage >&2; exit 2; }
	create_cluster
	;;
validate)
	[ "$#" -eq 1 ] || { usage >&2; exit 2; }
	guard_cluster
	revoke_refresh_access
	validate_baseline
	validate_refresh
	validate_phase_six_and_seven
	say "all Phase 4-7 Kubernetes and RBAC scenarios passed"
	;;
kubeconfigs)
	[ "$#" -le 2 ] || { usage >&2; exit 2; }
	guard_cluster
	export_kubeconfigs "${2:-$state_dir}"
	;;
app-e2e)
	[ "$#" -eq 2 ] || { usage >&2; exit 2; }
	run_app_e2e "$2"
	;;
refresh-grant)
	[ "$#" -eq 1 ] || { usage >&2; exit 2; }
	guard_cluster
	grant_refresh_access
	wait_can_i yes "$manual_subject" list pods kp-denied "temporary refresh grant is active"
	;;
refresh-revoke)
	[ "$#" -eq 1 ] || { usage >&2; exit 2; }
	guard_cluster
	revoke_refresh_access
	wait_can_i no "$manual_subject" list pods kp-denied "temporary refresh grant is revoked"
	;;
*)
	usage >&2
	exit 2
	;;
esac
