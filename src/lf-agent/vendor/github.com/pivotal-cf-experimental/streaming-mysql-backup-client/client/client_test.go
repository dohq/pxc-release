package client_test

import (
	"fmt"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-cf-experimental/streaming-mysql-backup-client/config"
	"github.com/pivotal-cf-experimental/streaming-mysql-backup-client/download"

	"io/ioutil"
	"os"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"github.com/pivotal-cf-experimental/streaming-mysql-backup-client/client"
	"github.com/pivotal-cf-experimental/streaming-mysql-backup-client/client/clientfakes"
	"github.com/pivotal-cf-experimental/streaming-mysql-backup-client/tarpit"
)

var _ = Describe("Streaming MySQL Backup Client", func() {
	var (
		outputDirectory    string
		backupClient       *client.Client
		rootConfig         *config.Config
		fakeDownloader     *clientfakes.FakeDownloader
		fakeBackupPreparer *clientfakes.FakeBackupPreparer
		tarClient          *tarpit.TarClient
		backupFileGlob     = `mysql-backup-*.tar.gpg`
		backupMetadataGlob = `mysql-backup-*.txt`
	)

	BeforeEach(func() {
		var err error
		outputDirectory, err = ioutil.TempDir(os.TempDir(), "backup-download-test")
		Expect(err).ToNot(HaveOccurred())

		logger := lagertest.NewTestLogger("backup-download-test")
		logger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.INFO))

		rootConfig = &config.Config{
			Urls:         []string{"node1"},
			TmpDir:       outputDirectory,
			OutputDir:    outputDirectory,
			Logger:       logger,
			SymmetricKey: "hello",
			MetadataFields: map[string]string{
				"compressed": "Y",
				"encrypted":  "Y",
			},
		}

		tarClient = tarpit.NewSystemTarClient()

		fakeBackupPreparer = &clientfakes.FakeBackupPreparer{}
		fakeBackupPreparer.CommandReturns(exec.Command("true"))

		fakeDownloader = &clientfakes.FakeDownloader{}

		fakeDownloader.DownloadBackupStub = func(url string, streamedWriter download.StreamedWriter) error {
			file, err := os.Open("fixtures/newtar.tar")
			Expect(err).ToNot(HaveOccurred())

			return streamedWriter.WriteStream(file)
		}
	})

	JustBeforeEach(func() {
		backupClient = client.NewClient(*rootConfig, tarClient, fakeBackupPreparer, fakeDownloader)
	})

	AfterEach(func() {
		os.RemoveAll(outputDirectory)
	})

	It("Downloaded a file", func() {
		expectFileToNotExist(filepath.Join(outputDirectory, backupFileGlob))
		expectFileToNotExist(filepath.Join(outputDirectory, backupMetadataGlob))

		Expect(backupClient.Execute()).To(Succeed())

		expectFileToExist(filepath.Join(outputDirectory, backupFileGlob))
		expectFileToExist(filepath.Join(outputDirectory, backupMetadataGlob))
	})

	It("Filled metadata file", func() {
		Expect(backupClient.Execute()).To(Succeed())
		expectFileToExist(filepath.Join(outputDirectory, backupMetadataGlob))
		files, _ := filepath.Glob(outputDirectory + "/" + backupMetadataGlob)
		data, err := ioutil.ReadFile(files[0])
		Expect(err).ToNot(HaveOccurred())

		backupMetadataStr := string(data)

		Expect(backupMetadataStr).To(ContainSubstring("uuid ="))
		Expect(backupMetadataStr).To(ContainSubstring("name ="))
		Expect(backupMetadataStr).To(ContainSubstring("tool_name ="))
		Expect(backupMetadataStr).To(ContainSubstring("tool_command ="))
		Expect(backupMetadataStr).To(ContainSubstring("tool_version ="))
		Expect(backupMetadataStr).To(ContainSubstring("ibbackup_version ="))
		Expect(backupMetadataStr).To(ContainSubstring("server_version ="))
		Expect(backupMetadataStr).To(ContainSubstring("start_time ="))
		Expect(backupMetadataStr).To(ContainSubstring("end_time ="))
		Expect(backupMetadataStr).To(ContainSubstring("compressed ="))
		Expect(backupMetadataStr).To(ContainSubstring("encrypted ="))
	})

	It("Sets values for keys in metadata file based on MetadataFields", func() {
		Expect(backupClient.Execute()).To(Succeed())
		expectFileToExist(filepath.Join(outputDirectory, backupMetadataGlob))
		files, _ := filepath.Glob(outputDirectory + "/" + backupMetadataGlob)
		data, err := ioutil.ReadFile(files[0])
		Expect(err).ToNot(HaveOccurred())
		backupMetadataStr := string(data)

		for key, val := range rootConfig.MetadataFields {
			Expect(backupMetadataStr).To(ContainSubstring(fmt.Sprintf("%s = %s", key, val)))
		}
	})

	Context("When there are multiple URLs", func() {
		BeforeEach(func() {
			rootConfig.Urls = []string{"node1", "node2", "node3"}
		})

		Context("When successful", func() {
			It("Creates a backup for each URL", func() {
				fakeBackupPreparer.CommandReturnsOnCall(0, exec.Command("true"))
				fakeBackupPreparer.CommandReturnsOnCall(1, exec.Command("true"))
				fakeBackupPreparer.CommandReturnsOnCall(2, exec.Command("true"))

				Expect(backupClient.Execute()).To(Succeed())

				matches, err := filepath.Glob(filepath.Join(outputDirectory, backupFileGlob))
				Expect(err).ToNot(HaveOccurred())
				Expect(matches).To(HaveLen(3))

				matches, err = filepath.Glob(filepath.Join(outputDirectory, backupMetadataGlob))
				Expect(err).ToNot(HaveOccurred())
				Expect(matches).To(HaveLen(3))
			})
		})

		Context("When unsuccessful", func() {
			Context("when all fail", func() {
				BeforeEach(func() {
					fakeBackupPreparer.CommandReturns(exec.Command("false"))
				})

				It("Returns the error", func() {
					err := backupClient.Execute()
					Expect(fakeBackupPreparer.CommandCallCount()).To(Equal(3))
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(MatchRegexp(`multiple errors:`))
					Expect(err).To(HaveLen(3))
				})
			})

			Context("when at least one is successful", func() {
				BeforeEach(func() {
					fakeBackupPreparer.CommandReturnsOnCall(0, exec.Command("false"))
					fakeBackupPreparer.CommandReturnsOnCall(1, exec.Command("false"))
					fakeBackupPreparer.CommandReturnsOnCall(2, exec.Command("true"))

					expectFileToNotExist(filepath.Join(outputDirectory, backupFileGlob))
					expectFileToNotExist(filepath.Join(outputDirectory, backupMetadataGlob))
				})

				It("Continues to create backups and exits successfully", func() {
					Expect(backupClient.Execute()).To(Succeed())

					Expect(fakeBackupPreparer.CommandCallCount()).To(Equal(3))

					matches, err := filepath.Glob(filepath.Join(outputDirectory, backupFileGlob))
					Expect(err).ToNot(HaveOccurred())
					Expect(matches).To(HaveLen(1))

					matches, err = filepath.Glob(filepath.Join(outputDirectory, backupMetadataGlob))
					Expect(err).ToNot(HaveOccurred())
					Expect(matches).To(HaveLen(1))
				})
			})
		})
	})

})

func expectFileToNotExist(glob string) {
	matches, err := filepath.Glob(glob)
	Expect(err).ToNot(HaveOccurred())
	Expect(matches).To(HaveLen(0), fmt.Sprintf("Expected no files to match glob: %s", glob))
}

func expectFileToExist(glob string) {
	matches, err := filepath.Glob(glob)
	Expect(err).ToNot(HaveOccurred())
	Expect(matches).ToNot(HaveLen(0), fmt.Sprintf("Expected at least one file to match glob: %s", glob))
}
