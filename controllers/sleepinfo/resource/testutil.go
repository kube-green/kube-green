package resource

import "context"

type ResourceMock struct {
	HasResourceResponseMock bool
	MockSleep               func(context.Context) error
	MockWakeUp              func(context.Context) error
	MockOriginalInfoToSave  func() ([]byte, error)
}

func (r ResourceMock) HasResource() bool {
	return r.HasResourceResponseMock
}

func (r ResourceMock) Sleep(ctx context.Context) error {
	if r.MockSleep == nil {
		return nil
	}
	return r.MockSleep(ctx)
}

func (r ResourceMock) WakeUp(ctx context.Context) error {
	if r.MockWakeUp == nil {
		return nil
	}
	return r.MockWakeUp(ctx)
}

func (r ResourceMock) GetOriginalInfoToSave() ([]byte, error) {
	if r.MockOriginalInfoToSave == nil {
		return nil, nil
	}
	return r.MockOriginalInfoToSave()
}

func GetResourceMock(mock ResourceMock) Resource {
	return mock
}
