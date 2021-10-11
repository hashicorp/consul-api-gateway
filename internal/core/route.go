package core

type CommonRoute struct {
	Meta      map[string]string
	Name      string
	Namespace string
}

func (c CommonRoute) GetMeta() map[string]string {
	return c.Meta
}

func (c CommonRoute) GetName() string {
	return c.Name
}

func (c CommonRoute) GetNamespace() string {
	return c.Namespace
}
