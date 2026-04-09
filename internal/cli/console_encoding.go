package cli

func configureConsoleUTF8(setCodePage func(codePage uint32, output bool) error) error {
	if err := setCodePage(65001, false); err != nil {
		return err
	}
	if err := setCodePage(65001, true); err != nil {
		return err
	}
	return nil
}
