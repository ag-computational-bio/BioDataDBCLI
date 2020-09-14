package cmd

/*
Copyright © 2020 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

import (
	"log"

	"github.com/ag-computational-bio/datahandlercli/upload"
	"github.com/spf13/cobra"
)

// uploadCmd represents the upload command
var uploadCmd = &cobra.Command{
	Use:   "upload",
	Short: "Uploads a single file into object storage and attaches it to the given dataset version",
	Long: `Uploads a single file into object storage and attaches it to the given dataset version
Uploads can only be performed using api tokens. Oauth2 logins are not supported.
The upload is performed using presigned links from s3. Depending on the filesize files are either uploaded in one go
or in multiple parts to avoid the 4GB limit during upload. Additionally the multi part upload can be performed in parallel.`,
	Run: func(cmd *cobra.Command, args []string) {
		token, err := cmd.Flags().GetString("token")
		if err != nil {
			log.Fatalln(err.Error())
		}

		filepaths, err := cmd.Flags().GetStringSlice("files")
		if err != nil {
			log.Fatalln(err.Error())
		}

		datasetVersionID, err := cmd.Flags().GetString("datasetversion")
		if err != nil {
			log.Fatalln(err.Error())
		}

		datasetID, err := cmd.Flags().GetString("dataset")
		if err != nil {
			log.Fatalln(err.Error())
		}

		err = upload.Upload(token, filepaths, datasetID, datasetVersionID)
		if err != nil {
			log.Fatalln(err.Error())
		}

	},
}

func init() {
	rootCmd.AddCommand(uploadCmd)
	uploadCmd.Flags().StringP("token", "t", "", "upload token")
	uploadCmd.Flags().StringSliceP("files", "f", []string{}, "files to upload")
	uploadCmd.Flags().StringP("datasetversion", "v", "", "datasetversion to associate the files with")
	uploadCmd.Flags().StringP("dataset", "d", "", "dataset to associate the files with")

	err := uploadCmd.MarkFlagRequired("token")
	if err != nil {
		log.Fatalln(err.Error())
	}

	err = uploadCmd.MarkFlagRequired("files")
	if err != nil {
		log.Fatalln(err.Error())
	}

	err = uploadCmd.MarkFlagRequired("dataset")
	if err != nil {
		log.Fatalln(err.Error())
	}
}
