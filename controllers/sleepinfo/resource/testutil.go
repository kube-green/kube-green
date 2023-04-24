package resource

import "context"

type Mock struct {
	HasResourceResponseMock bool
	MockSleep               func(context.Context) error
	MockWakeUp              func(context.Context) error
	MockOriginalInfoToSave  func() ([]byte, error)
}

func (r Mock) HasResource() bool {
	return r.HasResourceResponseMock
}

func (r Mock) Sleep(ctx context.Context) error {
	if r.MockSleep == nil {
		return nil
	}
	return r.MockSleep(ctx)
}

func (r Mock) WakeUp(ctx context.Context) error {
	if r.MockWakeUp == nil {
		return nil
	}
	return r.MockWakeUp(ctx)
}

func (r Mock) GetOriginalInfoToSave() ([]byte, error) {
	if r.MockOriginalInfoToSave == nil {
		return nil, nil
	}
	return r.MockOriginalInfoToSave()
}

func GetResourceMock(mock Mock) Resource {
	return mock
}
