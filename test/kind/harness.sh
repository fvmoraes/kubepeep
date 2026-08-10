#!/bin/sh
set -eu

umask 077

harness_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cluster_name=${KUBEPEEP_KIND_CLUSTER:-kubepeep-f4}
cluster_context=kind-$cluster_name
request_timeout=${KUBEPEEP_KIND_TIMEOUT:-15s}
fixture_file=$harness_dir/rbac.yaml
cluster_file=$harness_dir/cluster.yaml
state_dir=${KUBEPEEP_KIND_STATE_DIR:-$harness_dir/.state}
managed_value=kubepeep-kind-harness
manual_subject=system:serviceaccount:kp-harness:manual-viewer
lister_subject=system:serviceaccount:kp-harness:namespace-lister
dashboard_subject=system:serviceaccount:kp-harness:dashboard-viewer
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
  validate              validate single/list/all, denial and a reversible RBAC refresh
  kubeconfigs [DIR]     write short-lived restricted kubeconfigs (default: .state/)
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
	namespaced_resource_is_managed_or_absent role kp-allowed kp-resource-reader
	namespaced_resource_is_managed_or_absent role kp-denied kp-resource-reader
	namespaced_resource_is_managed_or_absent rolebinding kp-allowed kp-resource-readers
	namespaced_resource_is_managed_or_absent role kp-allowed kp-dashboard-log-reader
	namespaced_resource_is_managed_or_absent rolebinding kp-allowed kp-dashboard-log-reader
	namespaced_resource_is_managed_or_absent configmap kp-allowed kp-fixture
	namespaced_resource_is_managed_or_absent configmap kp-denied kp-fixture
	namespaced_resource_is_managed_or_absent pod kp-allowed kp-fixture
	namespaced_resource_is_managed_or_absent pod kp-denied kp-fixture
	namespaced_resource_is_managed_or_absent deployment kp-allowed kp-degraded
	namespaced_resource_is_managed_or_absent pod kp-allowed kp-restarting
	namespaced_resource_is_managed_or_absent event kp-allowed kp-warning
	kube apply --server-side --field-manager=kubepeep-kind-harness -f "$fixture_file"
}

can_i() {
	subject=$1
	verb=$2
	requested_resource=$3
	namespace=$4
	case "$requested_resource" in
	*/*)
		resource=${requested_resource%%/*}
		subresource=${requested_resource#*/}
		if [ "$namespace" = "-" ]; then
			kube auth can-i "$verb" "$resource" --subresource="$subresource" --as="$subject" >/dev/null 2>&1
		else
			kube auth can-i "$verb" "$resource" --subresource="$subresource" \
				--namespace="$namespace" --as="$subject" >/dev/null 2>&1
		fi
		;;
	*)
		resource=$requested_resource
		if [ "$namespace" = "-" ]; then
			kube auth can-i "$verb" "$resource" --as="$subject" >/dev/null 2>&1
		else
			kube auth can-i "$verb" "$resource" --namespace="$namespace" \
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

	if can_i "$subject" "$verb" "$requested_resource" "$namespace"; then
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
	attempt=0
	while [ "$attempt" -lt 30 ]; do
		if can_i "$subject" "$verb" "$requested_resource" "$namespace"; then
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
	kube get event kp-warning --namespace=kp-allowed --as="$dashboard_subject" -o name >/dev/null
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

export_kubeconfigs() {
	output_dir=${1:-$state_dir}
	mkdir -p -- "$output_dir"
	manual_output=$output_dir/manual-viewer.kubeconfig
	lister_output=$output_dir/namespace-lister.kubeconfig
	dashboard_output=$output_dir/dashboard-viewer.kubeconfig
	kubeconfig_target_is_safe "$manual_output" kubepeep-f4-manual
	kubeconfig_target_is_safe "$lister_output" kubepeep-f4-all
	kubeconfig_target_is_safe "$dashboard_output" kubepeep-f5-dashboard
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
	rm -rf -- "$temporary_dir"
	trap - 0 HUP INT TERM
	say "restricted, one-hour kubeconfigs written to $output_dir"
}

static_validate() {
	need python3
	if grep -Eq 'cluster-admin|resources:[[:space:]]*\[[^]]*"\*"|verbs:[[:space:]]*\[[^]]*"\*"' "$fixture_file"; then
		fail "fixture contains cluster-admin or wildcard RBAC"
	fi
	python3 - "$fixture_file" "$cluster_file" <<'PY'
import pathlib
import sys

try:
    import yaml
except ImportError as error:
    raise SystemExit("PyYAML is required for offline manifest validation") from error

fixture_path, cluster_path = map(pathlib.Path, sys.argv[1:])
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
    if document["kind"] in {"RoleBinding", "ClusterRoleBinding"}:
        if document.get("roleRef", {}).get("name") == "cluster-admin":
            raise SystemExit(f"fixture document {index} binds cluster-admin")

required = {
    ("Namespace", "", "kp-harness"),
    ("Namespace", "", "kp-allowed"),
    ("Namespace", "", "kp-denied"),
    ("ServiceAccount", "kp-harness", "manual-viewer"),
    ("ServiceAccount", "kp-harness", "namespace-lister"),
    ("ServiceAccount", "kp-harness", "dashboard-viewer"),
    ("Role", "kp-allowed", "kp-resource-reader"),
    ("Role", "kp-denied", "kp-resource-reader"),
    ("RoleBinding", "kp-allowed", "kp-resource-readers"),
    ("Role", "kp-allowed", "kp-dashboard-log-reader"),
    ("RoleBinding", "kp-allowed", "kp-dashboard-log-reader"),
    ("ClusterRole", "", "kp-namespace-lister"),
    ("ClusterRoleBinding", "", "kp-namespace-lister"),
    ("Pod", "kp-allowed", "kp-fixture"),
    ("Pod", "kp-denied", "kp-fixture"),
    ("Deployment", "kp-allowed", "kp-degraded"),
    ("Pod", "kp-allowed", "kp-restarting"),
    ("Event", "kp-allowed", "kp-warning"),
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
for document in documents:
    if document.get("kind") != "ClusterRoleBinding":
        continue
    for subject in document.get("subjects", []):
        if subject.get("name") == "manual-viewer":
            raise SystemExit("manual viewer must not have a ClusterRoleBinding")

dashboard_role = indexed[("Role", "kp-allowed", "kp-dashboard-log-reader")]
if dashboard_role.get("rules") != [{"apiGroups": [""], "resources": ["pods/log"], "verbs": ["get"]}]:
    raise SystemExit("dashboard log Role is broader than get pods/log")
warning = indexed[("Event", "kp-allowed", "kp-warning")]
if warning.get("type") != "Warning" or warning.get("count") != 3:
    raise SystemExit("dashboard Warning event fixture is not canonical")

cluster = yaml.safe_load(cluster_path.read_text(encoding="utf-8"))
if not isinstance(cluster, dict):
    raise SystemExit("Kind config is not a YAML mapping")
if cluster.get("apiVersion") != "kind.x-k8s.io/v1alpha4" or cluster.get("kind") != "Cluster":
    raise SystemExit("Kind config has an unexpected apiVersion/kind")
if not isinstance(cluster.get("nodes"), list) or not cluster["nodes"]:
    raise SystemExit("Kind config must declare at least one node")
PY
	say "manifests parse locally and contain no cluster-admin/wildcard grants"
}

create_cluster() {
	need kind
	need docker
	need kubectl
	docker info >/dev/null 2>&1 || fail "Docker daemon is unavailable; no cluster state was changed"
	if kind_cluster_exists; then
		say "reusing dedicated cluster $cluster_name (it will not be deleted)"
	else
		kind create cluster --name "$cluster_name" --config "$cluster_file" --wait 120s
	fi
	guard_cluster
	apply_fixtures
	revoke_refresh_access
	validate_baseline
	say "cluster is ready at context $cluster_context"
}

validate_cluster_name
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
	say "all Phase 4 RBAC scenarios passed"
	;;
kubeconfigs)
	[ "$#" -le 2 ] || { usage >&2; exit 2; }
	guard_cluster
	export_kubeconfigs "${2:-$state_dir}"
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
