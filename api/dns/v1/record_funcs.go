package v1

import "strings"

func (r *RecordList) Get(namespace, name string) *Record {
	for i := range r.Items {
		record := &r.Items[i]
		if record.Namespace == namespace && record.Name == name {
			return record
		}
	}
	return nil
}

func (r *RecordSpec) ExtraBool(key string) *bool {
	if value, ok := r.Extra[key]; ok {
		v := value == "true"
		return &v
	}
	return nil
}

func (r *RecordSpec) ExtraString(key string) *string {
	if value, ok := r.Extra[key]; ok {
		v := value
		return &v
	}
	return nil
}

func (r *RecordSpec) ExtraStrings(key string) []string {
	if value, ok := r.Extra[key]; ok {
		v := strings.Split(value, ",")
		return v
	}
	return nil
}

func (s *RecordStatus) FindProviderStatus(p NamespacedName) *RecordProviderStatus {
	for _, provider := range s.Providers {
		if provider.NamespacedName.Equal(&p) {
			return provider
		}
	}
	return nil
}

func (s *RecordProviderStatus) Success(id, data string) {
	s.RecordID = id
	s.Data = data
	s.Message = ""
}

func (s *RecordProviderStatus) Error(id, data string, err error) {
	s.RecordID = id
	s.Data = data
	s.Message = err.Error()
}
