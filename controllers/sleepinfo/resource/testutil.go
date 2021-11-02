package resource

import "context"

type ResourceMock struct {
	HasResourceResponseMock bool
	mockSleep               func(context.Context) error
	mockWakeUp              func(context.Context) error
	mockOriginalInfoToSave  func() ([]byte, error)
}

func (r ResourceMock) HasResource() bool {
	return r.HasResourceResponseMock
}

func (r ResourceMock) Sleep(ctx context.Context) error {
	return r.mockSleep(ctx)
}

func (r ResourceMock) WakeUp(ctx context.Context) error {
	return r.mockWakeUp(ctx)
}

func (r ResourceMock) GetOriginalInfoToSave() ([]byte, error) {
	return r.mockOriginalInfoToSave()
}

func GetResourceMock(mock ResourceMock) Resource {
	return mock
}
