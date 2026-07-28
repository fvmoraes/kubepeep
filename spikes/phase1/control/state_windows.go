//go:build windows

package control

import (
	"errors"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

const windowsFullControlMask = windows.STANDARD_RIGHTS_REQUIRED |
	windows.SYNCHRONIZE |
	0x1ff

func protectRuntimeDir(path string, _ os.FileInfo) error {
	if err := protectWindowsPath(
		path,
		windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT,
		true,
	); err != nil {
		return fmt.Errorf("control: protect Windows runtime directory: %w", err)
	}
	return nil
}

func validatePrivateState(path string, _ os.FileInfo) error {
	if err := validateWindowsPrivateDACL(path, false); err != nil {
		return fmt.Errorf("control: insecure Windows instance permissions: %w", err)
	}
	return nil
}

func protectTemporaryState(path string) error {
	return protectWindowsPath(path, windows.NO_INHERITANCE, false)
}

func atomicReplace(source, target string) error {
	if err := validateWindowsPrivateDACL(source, false); err != nil {
		return fmt.Errorf("validate private temporary state: %w", err)
	}
	sourcePtr, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	targetPtr, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(
		sourcePtr,
		targetPtr,
		windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH,
	)
}

func syncRuntimeDir(string) error {
	// MoveFileEx with MOVEFILE_WRITE_THROUGH is the durable boundary available
	// to this isolated Windows spike.
	return nil
}

func protectWindowsPath(path string, inheritance uint32, requireInheritance bool) error {
	if err := validateWindowsOwner(path); err != nil {
		return err
	}

	sid, err := currentWindowsUserSID()
	if err != nil {
		return err
	}
	acl, err := windows.ACLFromEntries(
		[]windows.EXPLICIT_ACCESS{{
			AccessPermissions: windows.GENERIC_ALL,
			AccessMode:        windows.SET_ACCESS,
			Inheritance:       inheritance,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_USER,
				TrusteeValue: windows.TrusteeValueFromSID(sid),
			},
		}},
		nil,
	)
	if err != nil {
		return fmt.Errorf("create current-user DACL: %w", err)
	}
	if err := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|
			windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		acl,
		nil,
	); err != nil {
		return fmt.Errorf("set current-user DACL: %w", err)
	}
	return validateWindowsPrivateDACL(path, requireInheritance)
}

func validateWindowsOwner(path string) error {
	sid, err := currentWindowsUserSID()
	if err != nil {
		return err
	}
	descriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION,
	)
	if err != nil {
		return fmt.Errorf("read owner: %w", err)
	}
	if descriptor == nil {
		return errors.New("missing security descriptor")
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		return fmt.Errorf("read owner SID: %w", err)
	}
	if owner == nil || !sid.Equals(owner) {
		return errors.New("path is not owned by the current user")
	}
	return nil
}

func validateWindowsPrivateDACL(path string, requireInheritance bool) error {
	return validateWindowsCurrentUserOnlyDACL(path, true, requireInheritance)
}

func validateWindowsInheritedDACL(path string) error {
	return validateWindowsCurrentUserOnlyDACL(path, false, false)
}

func validateWindowsCurrentUserOnlyDACL(
	path string,
	requireProtected bool,
	requireInheritance bool,
) error {
	sid, err := currentWindowsUserSID()
	if err != nil {
		return err
	}
	descriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return fmt.Errorf("read security descriptor: %w", err)
	}
	if descriptor == nil {
		return errors.New("missing security descriptor")
	}

	owner, _, err := descriptor.Owner()
	if err != nil {
		return fmt.Errorf("read owner SID: %w", err)
	}
	if owner == nil || !sid.Equals(owner) {
		return errors.New("path is not owned by the current user")
	}

	control, _, err := descriptor.Control()
	if err != nil {
		return fmt.Errorf("read security descriptor control: %w", err)
	}
	if requireProtected && control&windows.SE_DACL_PROTECTED == 0 {
		return errors.New("DACL inheritance is not protected")
	}

	dacl, _, err := descriptor.DACL()
	if err != nil {
		return fmt.Errorf("read DACL: %w", err)
	}
	if dacl == nil {
		return errors.New("DACL is empty")
	}
	if dacl.AceCount == 0 {
		return errors.New("DACL has no access rules")
	}

	appliesToPath := false
	propagatesToChildren := !requireInheritance
	for index := uint16(0); index < dacl.AceCount; index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(index), &ace); err != nil {
			return fmt.Errorf("read DACL ACE %d: %w", index, err)
		}
		if ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return fmt.Errorf("DACL ACE %d must allow access", index)
		}
		aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !aceSID.IsValid() || !sid.Equals(aceSID) {
			return fmt.Errorf("DACL ACE %d must belong to the current user", index)
		}
		if ace.Mask&windows.GENERIC_ALL == 0 &&
			ace.Mask&windowsFullControlMask != windowsFullControlMask {
			return fmt.Errorf(
				"DACL ACE %d does not grant full control: %#x",
				index,
				ace.Mask,
			)
		}
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE == 0 {
			appliesToPath = true
		}
		if ace.Header.AceFlags&windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT ==
			windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT {
			propagatesToChildren = true
		}
	}
	if !appliesToPath {
		return errors.New("DACL does not grant access to the path itself")
	}
	if !propagatesToChildren {
		return errors.New("runtime DACL does not propagate to child objects")
	}
	return nil
}

func currentWindowsUserSID() (*windows.SID, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("read current Windows user: %w", err)
	}
	if user == nil || user.User.Sid == nil || !user.User.Sid.IsValid() {
		return nil, errors.New("current Windows user has no valid SID")
	}
	return user.User.Sid, nil
}
