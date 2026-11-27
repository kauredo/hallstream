//go:build windows

package delegate

import (
	"sync"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
)

// Only a limited number of callbacks may be created in a single Go process,
// and any memory allocated for these callbacks is never released.
// Between NewCallback and NewCallbackCDecl, at least 1024 callbacks can always be created.
var (
	queryInterfaceCallback = syscall.NewCallback(queryInterface)
	addRefCallback         = syscall.NewCallback(addRef)
	releaseCallback        = syscall.NewCallback(release)
	// One Invoke callback per argument count. On 32-bit Windows the callee pops
	// its own arguments (stdcall), so a callback declaring more parameters than
	// the caller pushed corrupts the caller's stack. Patched in by Hallstream.
	invokeCallback0 = syscall.NewCallback(invoke0)
	invokeCallback2 = syscall.NewCallback(invoke2)
)

// noArgs is a delegate whose Invoke takes no arguments besides itself;
// every other delegate here takes two.
type noArgs interface{ InvokeTakesNoArgs() }

// Delegate represents a WinRT delegate class.
type Delegate interface {
	GetIID() *ole.GUID
	Invoke(instancePtr, rawArgs0, rawArgs1, rawArgs2, rawArgs3, rawArgs4, rawArgs5, rawArgs6, rawArgs7, rawArgs8 unsafe.Pointer) uintptr
	AddRef() uintptr
	Release() uintptr
}

// Callbacks contains the syscalls registered on Windows.
type Callbacks struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	Invoke         uintptr
}

var mutex = sync.RWMutex{}
var instances = make(map[uintptr]Delegate)

// RegisterCallbacks adds the given pointer and the Delegate it points to to our instances.
// This is required to redirect received callbacks to the correct object instance.
// The function returns the callbacks to use when creating a new delegate instance.
func RegisterCallbacks(ptr unsafe.Pointer, inst Delegate) *Callbacks {
	mutex.Lock()
	defer mutex.Unlock()
	instances[uintptr(ptr)] = inst

	c := &Callbacks{
		QueryInterface: queryInterfaceCallback,
		AddRef:         addRefCallback,
		Release:        releaseCallback,
		Invoke:         invokeCallback2,
	}
	if _, ok := inst.(noArgs); ok {
		c.Invoke = invokeCallback0
	}
	return c
}

func getInstance(ptr unsafe.Pointer) (Delegate, bool) {
	mutex.RLock() // locks writing, allows concurrent read
	defer mutex.RUnlock()

	i, ok := instances[uintptr(ptr)]
	return i, ok
}

func removeInstance(ptr unsafe.Pointer) {
	mutex.Lock()
	defer mutex.Unlock()
	delete(instances, uintptr(ptr))
}

func queryInterface(instancePtr unsafe.Pointer, iidPtr unsafe.Pointer, ppvObject *unsafe.Pointer) uintptr {
	instance, ok := getInstance(instancePtr)
	if !ok {
		// instance not found
		return ole.E_POINTER
	}

	// Checkout these sources for more information about the QueryInterface method.
	//   - https://docs.microsoft.com/en-us/cpp/atl/queryinterface
	//   - https://docs.microsoft.com/en-us/windows/win32/api/unknwn/nf-unknwn-iunknown-queryinterface(refiid_void)

	if ppvObject == nil {
		// If ppvObject (the address) is nullptr, then this method returns E_POINTER.
		return ole.E_POINTER
	}

	// This function must adhere to the QueryInterface defined here:
	// https://docs.microsoft.com/en-us/windows/win32/api/unknwn/nn-unknwn-iunknown
	if iid := (*ole.GUID)(iidPtr); ole.IsEqualGUID(iid, instance.GetIID()) || ole.IsEqualGUID(iid, ole.IID_IUnknown) || ole.IsEqualGUID(iid, ole.IID_IInspectable) {
		*ppvObject = instancePtr
	} else {
		*ppvObject = nil
		// Return E_NOINTERFACE if the interface is not supported
		return ole.E_NOINTERFACE
	}

	// If the COM object implements the interface, then it returns
	// a pointer to that interface after calling IUnknown::AddRef on it.
	(*ole.IUnknown)(*ppvObject).AddRef()

	// Return S_OK if the interface is supported
	return ole.S_OK
}

func invoke0(instancePtr unsafe.Pointer) uintptr {
	return invoke(instancePtr, nil, nil)
}

func invoke2(instancePtr, rawArgs0, rawArgs1 unsafe.Pointer) uintptr {
	return invoke(instancePtr, rawArgs0, rawArgs1)
}

func invoke(instancePtr, rawArgs0, rawArgs1 unsafe.Pointer) uintptr {
	instance, ok := getInstance(instancePtr)
	if !ok {
		// instance not found
		return ole.E_FAIL
	}

	return instance.Invoke(instancePtr, rawArgs0, rawArgs1, nil, nil, nil, nil, nil, nil, nil)
}

func addRef(instancePtr unsafe.Pointer) uintptr {
	instance, ok := getInstance(instancePtr)
	if !ok {
		// instance not found
		return ole.E_FAIL
	}

	return instance.AddRef()
}

func release(instancePtr unsafe.Pointer) uintptr {
	instance, ok := getInstance(instancePtr)
	if !ok {
		// instance not found
		return ole.E_FAIL
	}

	rem := instance.Release()
	if rem == 0 {
		// remove this delegate
		removeInstance(instancePtr)
	}
	return rem
}
