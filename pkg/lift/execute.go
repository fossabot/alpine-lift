package lift

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	log "github.com/sirupsen/logrus"
)

const (
	drpcliBin    = "/usr/local/bin/drpcli"
	drpcliRCFile = "/etc/init.d/drpcli"
)

// executes the setup-alpine script, using a generated answerfile
func (l *Lift) alpineSetup() error {
	f, err := generateFileFromTemplate(*answerFile, l.Data)
	if err != nil {
		return err
	}
	cmd := exec.Command("setup-alpine", "-f", f)
	// setup-alpine script asks for root password on stdin
	input := []byte(fmt.Sprintf("%s\n%s\n", l.Data.RootPasswd, l.Data.RootPasswd))
	cmd.Stdin = bytes.NewBuffer(input)
	// Show setup-alpine output on stdout
	cmd.Stdout = os.Stdout
	// Ignore any errors, since exit code can be 1 if
	// e.g. service is already running.
	_ = cmd.Run()
	return nil
}

// parses sshd_config, writes authorized_keys file and restarts sshd service
func (l *Lift) sshdSetup() error {
	if err := parseConfigFile("/etc/ssh/sshd_config", " ", l.getSSHDKVMap()); err != nil {
		return err
	}
	if err := l.addSSHKeys(); err != nil {
		return err
	}
	if err := doService("sshd", RESTART); err != nil {
		return err
	}
	return nil
}

// opens or creates authorized_keys file, and adds ssh keys
// from alpine-data
func (l *Lift) addSSHKeys() error {
	if l.Data.SSHDConfig.AuthorizedKeys != nil && len(l.Data.SSHDConfig.AuthorizedKeys) > 0 {
		file, err := openOrCreate("/root/.ssh/authorized_keys")
		if err != nil {
			return err
		}
		defer file.Close()
		for _, key := range l.Data.SSHDConfig.AuthorizedKeys {
			if _, err = file.WriteString(fmt.Sprintf("%s\n", key)); err != nil {
				return err
			}
		}
	}
	return nil
}

// downloads drpcli and installs it as a service
func (l *Lift) drpSetup() error {
	// First download drpcli
	if _, err := os.Stat(drpcliBin); os.IsNotExist(err) {
		url := fmt.Sprintf("%s/drpcli.amd64.linux", l.Data.DRP.AssetsURL)
		log.WithField("url", url).Debug("Downloading drpcli")
		drpcli, err := downloadFile(url)
		if err != nil {
			return err
		}
		log.Debugf("Saving drpcli to %s", drpcliBin)
		err = ioutil.WriteFile(drpcliBin, drpcli, 0755)
		if err != nil {
			return err
		}
	}

	// then check RC file
	if _, err := os.Stat(drpcliRCFile); os.IsNotExist(err) {
		log.Debug("Generating drpcli rc service file")
		rcfile, err := generateFileFromTemplate(*drpcliInit, l.Data)
		if err != nil {
			return err
		}
		log.Debugf("Copying service file to %s", drpcliRCFile)
		cmd := exec.Command("cp", rcfile, drpcliRCFile)
		err = cmd.Run()
		if err != nil {
			return err
		}
		log.Debug("Setting execute permission")
		cmd = exec.Command("chmod", "+x", drpcliRCFile)
		err = cmd.Run()
		if err != nil {
			return err
		}
		log.Debug("Add drpcli service to default runlevel")
		cmd = exec.Command("rc-update", "add", "drpcli")
		err = cmd.Run()
		if err != nil {
			return err
		}
	}

	log.Info("Starting dr-provision runner")
	_ = doService("drpcli", START)
	return nil
}
