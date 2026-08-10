//go:build windows

// Package winacl centralizes the current-token-user-only Windows DACL policy.
package winacl

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

const fileAllAccess windows.ACCESS_MASK = 0x001f01ff

func CurrentUserSID() (*windows.SID, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	if user == nil || user.User.Sid == nil {
		return nil, errors.New("current token user SID is unavailable")
	}
	return user.User.Sid, nil
}

func Build(directory bool) (*windows.ACL, error) {
	user, err := CurrentUserSID()
	if err != nil {
		return nil, err
	}
	ace := "(A;;GA;;;" + user.String() + ")"
	if directory {
		ace = "(A;OICI;GA;;;" + user.String() + ")"
	}
	descriptor, err := windows.SecurityDescriptorFromString("D:P" + ace)
	if err != nil {
		return nil, err
	}
	dacl, _, err := descriptor.DACL()
	return dacl, err
}

func SetHandle(handle windows.Handle, directory bool) error {
	dacl, err := Build(directory)
	if err != nil {
		return err
	}
	return windows.SetSecurityInfo(
		handle, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil,
	)
}

func SetNamed(path string, directory bool) error {
	dacl, err := Build(directory)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil,
	)
}

// Validate checks the effective security property instead of requiring one
// byte-for-byte ACL encoding. Windows may canonicalize one inheritable full-
// control entry into multiple allow ACEs for the same SID. Every ACE must
// still belong to the current token user; broad/system/admin/unknown entries,
// deny/object ACEs and incomplete rights are rejected.
func Validate(handle windows.Handle, directory, requireProtected bool) error {
	descriptor, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	control, _, err := descriptor.Control()
	if err != nil {
		return err
	}
	if control&windows.SE_DACL_PRESENT == 0 || requireProtected && control&windows.SE_DACL_PROTECTED == 0 {
		return errors.New("DACL is absent or not protected")
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	if dacl == nil || dacl.AceCount == 0 || dacl.AceCount > 64 {
		return errors.New("DACL has no bounded private entries")
	}
	user, err := CurrentUserSID()
	if err != nil {
		return err
	}
	var directMask, inheritedObjectMask, inheritedContainerMask windows.ACCESS_MASK
	allowedFlags := uint8(windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE | windows.INHERIT_ONLY_ACE | windows.INHERITED_ACE)
	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil {
			return err
		}
		if ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errors.New("DACL contains a non-allow entry")
		}
		if ace.Header.AceFlags&^allowedFlags != 0 {
			return errors.New("DACL contains unsupported inheritance flags")
		}
		aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !windows.EqualSid(user, aceSID) {
			return errors.New("DACL grants access to another principal")
		}
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE == 0 {
			directMask |= ace.Mask
		}
		if ace.Header.AceFlags&windows.OBJECT_INHERIT_ACE != 0 {
			inheritedObjectMask |= ace.Mask
		}
		if ace.Header.AceFlags&windows.CONTAINER_INHERIT_ACE != 0 {
			inheritedContainerMask |= ace.Mask
		}
	}
	if !grantsFullControl(directMask) {
		return errors.New("DACL does not grant the current user full control")
	}
	if directory && (!grantsFullControl(inheritedObjectMask) || !grantsFullControl(inheritedContainerMask)) {
		return errors.New("directory DACL does not safely inherit full control")
	}
	return nil
}

func grantsFullControl(mask windows.ACCESS_MASK) bool {
	return mask&windows.GENERIC_ALL != 0 || mask&fileAllAccess == fileAllAccess
}
