package redis

func GetRedisError(err error) error {
	if err == nil || err.Error() == "redis: nil" {
		return nil
	} else {
		return err
	}
}
