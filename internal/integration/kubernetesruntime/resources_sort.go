package kubernetesruntime

import (
	"strings"

	"github.com/fvmoraes/kubepeep/internal/services/resources"
)

// naturalTextCompare compares text bytewise, except that contiguous ASCII
// digit runs are compared by their numeric value. It deliberately does not
// fold case or parse integers, so the result is deterministic and cannot
// overflow for long Kubernetes-generated name suffixes.
func naturalTextCompare(left, right string) int {
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left) && rightIndex < len(right) {
		leftDigit := isASCIIDigit(left[leftIndex])
		rightDigit := isASCIIDigit(right[rightIndex])
		if leftDigit && rightDigit {
			leftEnd := digitRunEnd(left, leftIndex)
			rightEnd := digitRunEnd(right, rightIndex)
			leftSignificant := significantDigitStart(left, leftIndex, leftEnd)
			rightSignificant := significantDigitStart(right, rightIndex, rightEnd)

			leftLength := leftEnd - leftSignificant
			rightLength := rightEnd - rightSignificant
			if leftLength < rightLength {
				return -1
			}
			if leftLength > rightLength {
				return 1
			}
			if comparison := strings.Compare(left[leftSignificant:leftEnd], right[rightSignificant:rightEnd]); comparison != 0 {
				return comparison
			}

			// Numerically equivalent runs (for example 2 and 02) remain
			// equivalent here. The collection's canonical identity is the
			// deterministic ascending tie-breaker.
			leftIndex, rightIndex = leftEnd, rightEnd
			continue
		}

		if left[leftIndex] < right[rightIndex] {
			return -1
		}
		if left[leftIndex] > right[rightIndex] {
			return 1
		}
		leftIndex++
		rightIndex++
	}
	if leftIndex < len(left) {
		return 1
	}
	if rightIndex < len(right) {
		return -1
	}
	return 0
}

func naturalTextFieldsCompare(left, right []string) int {
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	for index := 0; index < limit; index++ {
		if comparison := naturalTextCompare(left[index], right[index]); comparison != 0 {
			return comparison
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

func lexicalTextFieldsCompare(left, right []string) int {
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	for index := 0; index < limit; index++ {
		if comparison := strings.Compare(left[index], right[index]); comparison != 0 {
			return comparison
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

func int64Compare(left, right int64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

// pageSortLess applies direction only to the requested primary value. A
// primary tie always uses the collection's canonical identity ascending,
// which keeps descending results stable and deterministic.
func pageSortLess(primary, canonical int, descending bool) bool {
	if primary == 0 {
		return canonical < 0
	}
	if descending {
		return primary > 0
	}
	return primary < 0
}

func workloadPageIdentityCompare(left, right resources.WorkloadDTO) int {
	leftRank, rightRank := workloadKindRank(left.Kind), workloadKindRank(right.Kind)
	if leftRank < rightRank {
		return -1
	}
	if leftRank > rightRank {
		return 1
	}
	return naturalTextFieldsCompare(
		[]string{left.Kind, left.Namespace, left.Name},
		[]string{right.Kind, right.Namespace, right.Name},
	)
}

func workloadPageCanonicalCompare(left, right resources.WorkloadDTO) int {
	leftRank, rightRank := workloadKindRank(left.Kind), workloadKindRank(right.Kind)
	if leftRank < rightRank {
		return -1
	}
	if leftRank > rightRank {
		return 1
	}
	return lexicalTextFieldsCompare(
		[]string{left.Kind, left.Namespace, left.Name},
		[]string{right.Kind, right.Namespace, right.Name},
	)
}

func podPageIdentityCompare(left, right resources.PodDTO) int {
	return naturalTextFieldsCompare(
		[]string{left.Namespace, left.Name},
		[]string{right.Namespace, right.Name},
	)
}

func podPageCanonicalCompare(left, right resources.PodDTO) int {
	return lexicalTextFieldsCompare(
		[]string{left.Namespace, left.Name},
		[]string{right.Namespace, right.Name},
	)
}

func eventPageIdentityCompare(left, right resources.EventDTO) int {
	return naturalTextFieldsCompare(
		[]string{left.Namespace, left.ObjectKind, left.ObjectName, left.Reason},
		[]string{right.Namespace, right.ObjectKind, right.ObjectName, right.Reason},
	)
}

func eventPageCanonicalCompare(left, right resources.EventDTO) int {
	if comparison := lexicalTextFieldsCompare(
		[]string{left.Namespace, left.ObjectKind, left.ObjectName, left.Reason, pointerString(left.Timestamp), left.Type, pointerString(left.Source), left.Message},
		[]string{right.Namespace, right.ObjectKind, right.ObjectName, right.Reason, pointerString(right.Timestamp), right.Type, pointerString(right.Source), right.Message},
	); comparison != 0 {
		return comparison
	}
	return int64Compare(left.Count, right.Count)
}

func servicePageIdentityCompare(left, right resources.ServiceDTO) int {
	return naturalTextFieldsCompare(
		[]string{left.Namespace, left.Name},
		[]string{right.Namespace, right.Name},
	)
}

func servicePageCanonicalCompare(left, right resources.ServiceDTO) int {
	return lexicalTextFieldsCompare(
		[]string{left.Namespace, left.Name},
		[]string{right.Namespace, right.Name},
	)
}

func ingressPageIdentityCompare(left, right resources.IngressDTO) int {
	return naturalTextFieldsCompare(
		[]string{left.Namespace, left.Name},
		[]string{right.Namespace, right.Name},
	)
}

func ingressPageCanonicalCompare(left, right resources.IngressDTO) int {
	return lexicalTextFieldsCompare(
		[]string{left.Namespace, left.Name},
		[]string{right.Namespace, right.Name},
	)
}

func endpointSlicePageIdentityCompare(left, right resources.EndpointSliceDTO) int {
	return naturalTextFieldsCompare(
		[]string{left.Namespace, left.Name},
		[]string{right.Namespace, right.Name},
	)
}

func endpointSlicePageCanonicalCompare(left, right resources.EndpointSliceDTO) int {
	return lexicalTextFieldsCompare(
		[]string{left.Namespace, left.Name},
		[]string{right.Namespace, right.Name},
	)
}

func configMapPageIdentityCompare(left, right resources.ConfigMapListDTO) int {
	return naturalTextFieldsCompare(
		[]string{left.Namespace, left.Name, left.UID},
		[]string{right.Namespace, right.Name, right.UID},
	)
}

func configMapPageCanonicalCompare(left, right resources.ConfigMapListDTO) int {
	return lexicalTextFieldsCompare(
		[]string{left.Namespace, left.Name, left.UID},
		[]string{right.Namespace, right.Name, right.UID},
	)
}

func secretPageIdentityCompare(left, right resources.SecretMetadataDTO) int {
	return naturalTextFieldsCompare(
		[]string{left.Metadata.Namespace, left.Metadata.Name, left.Metadata.UID},
		[]string{right.Metadata.Namespace, right.Metadata.Name, right.Metadata.UID},
	)
}

func secretPageCanonicalCompare(left, right resources.SecretMetadataDTO) int {
	return lexicalTextFieldsCompare(
		[]string{left.Metadata.Namespace, left.Metadata.Name, left.Metadata.UID},
		[]string{right.Metadata.Namespace, right.Metadata.Name, right.Metadata.UID},
	)
}

func isASCIIDigit(value byte) bool { return value >= '0' && value <= '9' }

func digitRunEnd(value string, start int) int {
	end := start
	for end < len(value) && isASCIIDigit(value[end]) {
		end++
	}
	return end
}

func significantDigitStart(value string, start, end int) int {
	for start < end && value[start] == '0' {
		start++
	}
	return start
}
