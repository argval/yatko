use std::fmt;

use bitflags::bitflags;

bitflags! {
    /// <https://github.com/jrsoftware/issrc/blob/is-6_5_1/Projects/Src/Shared.Struct.pas#L256>
    #[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
    pub struct FileFlags: u64 {
        /// Always ask the user to confirm before replacing an existing file.
        #[doc(alias = "confirmoverwrite")]
        const CONFIRM_OVERWRITE = 1;

        /// Never remove the file. This flag can be useful when installing very common shared files
        /// that shouldn't be deleted under any circumstances, such as MFC DLLs.
        ///
        /// Note that if this flag is combined with the [`sharedfile`] flag, the file will never be
        /// deleted at uninstall time but the reference count will still be properly decremented.
        ///
        /// [`sharedfile`]: FileFlags::SHARED_FILE
        #[doc(alias = "uninsneveruninstall")]
        const NEVER_UNINSTALL = 1 << 1;

        /// When an existing file needs to be replaced, and it is in use (locked) by another running
        /// process, Setup will by default display an error message. This flag tells Setup to
        /// instead register the file to be replaced the next time the system is restarted (by
        /// calling MoveFileEx or by creating an entry in WININIT.INI). When this happens, the user
        /// will be prompted to restart their computer at the end of the installation process.
        ///
        /// NOTE: This flag has no effect if the user does not have administrative privileges.
        /// Therefore, when using this flag, it is recommended that you leave the
        /// [PrivilegesRequired] `[Setup]` section directive at the default setting of admin.
        ///
        /// [PrivilegesRequired]: https://jrsoftware.org/ishelp/index.php?topic=setup_privilegesrequired
        #[doc(alias = "restartreplace")]
        const RESTART_REPLACE = 1 << 2;

        /// Instructs Setup to install the file as usual, but then delete it once the installation
        /// is completed (or aborted). This can be useful for extracting temporary data needed by a
        /// program executed in the script's `[Run]` section.
        ///
        /// This flag will not cause existing files that weren't replaced during installation to be
        /// deleted.
        ///
        /// This flag cannot be combined with the [`isreadme`], [`regserver`], [`regtypelib`],
        /// [`restartreplace`], [`sharedfile`], or [`uninsneveruninstall`] flags.
        ///
        /// [`isreadme`]: FileFlags::IS_README_FILE
        /// [`regserver`]: FileFlags::REGISTER_SERVER
        /// [`regtypelib`]: FileFlags::REGISTER_TYPE_LIB
        /// [`restartreplace`]: FileFlags::RESTART_REPLACE
        /// [`sharedfile`]: FileFlags::SHARED_FILE
        /// [`uninsneveruninstall`]: FileFlags::NEVER_UNINSTALL
        #[doc(alias = "deleteafterinstall")]
        const DELETE_AFTER_INSTALL = 1 << 3;

        /// Register the DLL/OCX file. With this flag set, Setup will call the DllRegisterServer
        /// function exported by the DLL/OCX file, and the uninstaller will call DllUnregisterServer
        /// prior to removing the file. When used in combination with [`sharedfile`], the DLL/OCX
        /// file will only be unregistered when the reference count reaches zero.
        ///
        /// In [64-bit install mode], the file is assumed to be a 64-bit image and will be
        /// registered inside a 64-bit process. You can override this by specifying the [`32bit`]
        /// flag.
        ///
        /// [`sharedfile`]: FileFlags::SHARED_FILE
        /// [64-bit install mode]: https://jrsoftware.org/ishelp/topic_32vs64bitinstalls.htm
        /// [`32bit`]: FileFlags::BITS_32
        #[doc(alias = "regserver")]
        const REGISTER_SERVER = 1 << 4;

        /// Register the type library (.tlb). The uninstaller will unregister the type library
        /// (unless the flag [`uninsneveruninstall`] is specified). As with the [`regserver`] flag,
        /// when used in combination with [`sharedfile`], the file will only be unregistered by the
        /// uninstaller when the reference count reaches zero.
        ///
        /// In [64-bit install mode] running on an x64-compatible edition of Windows, the type
        /// library will be registered inside a 64-bit process. You can override this by specifying
        /// the [`32bit`] flag.
        ///
        /// [`uninsneveruninstall`]: FileFlags::NEVER_UNINSTALL
        /// [`regserver`]: FileFlags::REGISTER_SERVER
        /// [`sharedfile`]: FileFlags::SHARED_FILE
        /// [64-bit install mode]: https://jrsoftware.org/ishelp/topic_32vs64bitinstalls.htm
        /// [`32bit`]: FileFlags::BITS_32
        #[doc(alias = "regtypelib")]
        const REGISTER_TYPE_LIB = 1 << 5;

        /// Specifies that the file is shared among multiple applications, and should only be
        /// removed at uninstall time if no other applications are using it. Most files installed
        /// to the Windows System directory should use this flag, including .OCX, .BPL, and .DPL
        /// files.
        ///
        /// Windows' standard shared file reference-counting mechanism (located in the registry
        /// under HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\SharedDLLs) is used
        /// to keep track of how many applications depend on the file. Each time the file is
        /// installed, the reference count for the file is incremented. (This happens regardless of
        /// whether the installer actually replaces the file on disk.) When an application using the
        /// file is uninstalled, the reference count is decremented. If the count reaches zero, the
        /// file is deleted (with the user's confirmation, unless the [`uninsnosharedfileprompt`]
        /// flag is also specified).
        ///
        /// If Setup is run more than once, the reference count for the file will be incremented
        /// more than once. The uninstaller will decrement the reference count the same number of
        /// times, however, so no references are leaked (provided the [UninstallLogMode] `[Setup]`
        /// section directive isn't changed from its default setting of `append`).
        ///
        /// When this flag is used, do not specify `{syswow64}` in the `DestDir` parameter; use
        /// `{sys}` instead. Even though `{sys}` and `{syswow64}` map to the same underlying
        /// directory in [32-bit install mode], the path name must exactly match what every other
        /// existing installer is using; otherwise, a second reference count for the file would be
        /// created, which could result in the file being removed prematurely. If you need to
        /// install a shared file to the 32-bit System directory in [64-bit install mode], specify
        /// `{sys}` in the `DestDir` parameter and additionally include the [`32bit`] flag.
        ///
        /// [`uninsnosharedfileprompt`]: FileFlags::UNINS_NO_SHARED_FILE_PROMPT
        /// [UninstallLogMode]: https://jrsoftware.org/ishelp/topic_setup_uninstalllogmode.htm
        /// [32-bit install mode]: https://jrsoftware.org/ishelp/topic_32vs64bitinstalls.htm
        /// [64-bit install mode]: https://jrsoftware.org/ishelp/topic_32vs64bitinstalls.htm
        /// [`32bit`]: FileFlags::BITS_32
        #[doc(alias = "sharedfile")]
        const SHARED_FILE = 1 << 6;

        /// Instructs Setup to proceed to comparing time stamps (last write/modified time) if the
        /// file being installed already exists on the user's system, and at least one of the
        /// following conditions is true:
        ///
        /// * Neither the existing file nor the file being installed has version info.
        /// * The [`ignoreversion`] flag is also used on the entry.
        /// * The [`replacesameversion`] flag isn't used, and the existing file and the file being
        /// installed have the same version number (as determined by the files' version info).
        ///
        /// If the existing file has an older time stamp (last write/modified time) than the file
        /// being installed, the existing file will be replaced. Otherwise, it will not be replaced.
        ///
        /// Use of this flag is *not recommended* except as a last resort, because there is an
        /// inherent issue with it: NTFS partitions store time stamps in UTC (unlike FAT
        /// partitions), which causes local time stamps -- what Inno Setup works with by default --
        /// to shift whenever a user changes their system's time zone or when daylight saving time
        /// goes into or out of effect. This can create a situation where files are replaced when
        /// the user doesn't expect them to be, or not replaced when the user expects them to be.
        ///
        /// [`ignoreversion`]: FileFlags::IGNORE_VERSION
        /// [`replacesameversion`]: FileFlags::REPLACE_SAME_VERSION_IF_CONTENTS_DIFFER
        #[doc(alias = "comparetimestamp")]
        const COMPARE_TIME_STAMP = 1 << 7;

        /// Specify this flag if the entry is installing a *non-TrueType* font with the
        /// `FontInstall` parameter.
        #[doc(alias = "fontisnttruetype")]
        const FONT_IS_NOT_TRUE_TYPE = 1 << 8;

        /// This flag instructs the compiler -- or Setup, if the `external` flag is also used -- to
        /// silently skip over the entry if the source file does not exist, instead of displaying an
        /// error message.
        #[doc(alias = "skipifsourcedoesntexist")]
        const SKIP_IF_SOURCE_DOESNT_EXIST = 1 << 9;

        /// Always overwrite a read-only file. Without this flag, Setup will ask the user if an
        /// existing read-only file should be overwritten.
        #[doc(alias = "overwritereadonly")]
        const OVERWRITE_READ_ONLY = 1 << 10;

        /// When this flag is used and the file already exists on the user's system, and it has the
        /// same version number as the file being installed, Setup will compare the files and
        /// replace the existing file if their contents differ.
        ///
        /// The default behavior (i.e. when this flag isn't used) is to not replace an existing file
        /// with the same version number.
        #[doc(alias = "replacesameversion")]
        const OVERWRITE_SAME_VERSION = 1 << 11;

        const CUSTOM_DEST_NAME = 1 << 12;

        /// Only install the file if a file of the same name already exists on the user's system.
        /// This flag may be useful if your installation is a patch to an existing installation, and
        /// you don't want files to be installed that the user didn't already have.
        #[doc(alias = "onlyifdestfileexists")]
        const ONLY_IF_DEST_FILE_EXISTS = 1 << 13;

        /// When combined with either the [`regserver`] or [`regtypelib`] flags, Setup will not
        /// display any error message if the registration fails.
        ///
        /// [`regserver`]: FileFlags::REGISTER_SERVER
        /// [`regtypelib`]: FileFlags::REGISTER_TYPE_LIB
        #[doc(alias = "noregerror")]
        const NO_REG_ERROR = 1 << 14;

        /// When this flag is used and the file is in use at uninstall time, the uninstaller will
        /// queue the file to be deleted when the system is restarted, and at the end of the
        /// uninstallation process ask the user if they want to restart. This flag can be useful
        /// when uninstalling things like shell extensions which cannot be programmatically stopped.
        /// Note that administrative privileges are required for this flag to have an effect.
        #[doc(alias = "uninsrestartdelete")]
        const UNINS_RESTART_DELETE = 1 << 15;

        /// Only install the file if it doesn't already exist on the user's system.
        #[doc(alias = "onlyifdoesntexist")]
        const ONLY_IF_DOESNT_EXIST = 1 << 16;

        /// Don't compare version info at all; replace existing files regardless of their version
        /// number.
        ///
        /// This flag should only be used on files private to your application, *never* on shared
        /// system files.
        #[doc(alias = "ignoreversion")]
        const IGNORE_VERSION = 1 << 17;

        /// By default, when a file being installed has an older version number (or older time
        /// stamp, when the [`comparetimestamp`] flag is used) than an existing file, Setup will not
        /// replace the existing file. When this flag is used, Setup will ask the user whether the
        /// file should be replaced, with the default answer being to keep the existing file.
        ///
        /// [`comparetimestamp`]: FileFlags::COMPARE_TIME_STAMP
        #[doc(alias = "promptifolder")]
        const PROMPT_IF_OLDER = 1 << 18;

        /// Don't copy the file to the user's system during the normal file copying stage but do
        /// statically compile the file into the installation. This flag is useful if the file is
        /// handled by the `[Code]` section exclusively and extracted using [ExtractTemporaryFile].
        ///
        /// [ExtractTemporaryFile]: https://jrsoftware.org/ishelp/topic_isxfunc_extracttemporaryfile.htm
        #[doc(alias = "dontcopy")]
        const DONT_COPY = 1 << 19;

        /// When uninstalling the file, remove any read-only attribute from the file before
        /// attempting to delete it.
        #[doc(alias = "uninsremovereadonly")]
        const UNINS_REMOVE_READ_ONLY = 1 << 20;

        /// Instructs the compiler or Setup to also search for the `Source` filename/wildcard in
        /// subdirectories under the `Source` directory.
        #[doc(alias = "recursesubdirs")]
        const RECURSE_SUB_DIRS_EXTERNAL = 1 << 21;

        /// When this flag is used and the file already exists on the user's system, and it has the
        /// same version number as the file being installed, Setup will compare the files and
        /// replace the existing file if their contents differ.
        ///
        /// The default behavior (i.e. when this flag isn't used) is to not replace an existing file
        /// with the same version number.
        #[doc(alias = "replacesameversion")]
        const REPLACE_SAME_VERSION_IF_CONTENTS_DIFFER = 1 << 22;

        /// Prevents Setup from verifying the file checksum after extraction. Use this flag on files
        /// you wish to modify while already compiled into Setup.
        ///
        /// Must be combined with `nocompression`.
        #[doc(alias = "dontverifychecksum")]
        const DONT_VERIFY_CHECKSUM = 1 << 23;

        /// When uninstalling the shared file, automatically remove the file if its reference count
        /// reaches zero instead of asking the user. Must be combined with the [`sharedfile`] flag
        /// to have an effect.
        ///
        /// [`sharedfile`]: FileFlags::SHARED_FILE
        #[doc(alias = "uninsnosharedfileprompt")]
        const UNINS_NO_SHARED_FILE_PROMPT = 1 << 24;

        /// By default, the compiler skips empty directories when it recurses subdirectories
        /// searching for the Source filename/wildcard. This flag causes these directories to be
        /// created at install time (just like if you created `[Dirs]` entries for them).
        ///
        /// Must be combined with [`recursesubdirs`].
        ///
        /// [`recursesubdirs`]: FileFlags::RECURSE_SUB_DIRS_EXTERNAL
        #[doc(alias = "createallsubdirs")]
        const CREATE_ALL_SUB_DIRS = 1 << 25;

        /// Causes the `{sys}` constant to map to the 32-bit System directory when used in the
        /// `Source` and `DestDir` parameters, the [`regserver`] and [`regtypelib`] flags to treat
        /// the file as 32-bit, and the [`sharedfile`] flag to update the 32-bit SharedDLLs registry
        /// key. This is the default behavior in [32-bit install mode].
        ///
        /// [`regserver`]: FileFlags::REGISTER_SERVER
        /// [`regtypelib`]: FileFlags::REGISTER_TYPE_LIB
        /// [`sharedfile`]: FileFlags::SHARED_FILE
        /// [32-bit install mode]: https://jrsoftware.org/ishelp/topic_32vs64bitinstalls.htm
        #[doc(alias = "32bit")]
        const BITS_32 = 1 << 26;

        /// Causes the `{sys}` constant to map to the 64-bit System directory when used in the
        /// `Source` and `DestDir` parameters, the [`regserver`] and [`regtypelib`] flags to treat
        /// the file as 64-bit, and the sharedfile flag to update the 64-bit SharedDLLs registry
        /// key. This is the default behavior in [64-bit install mode].
        ///
        /// [`regserver`]: FileFlags::REGISTER_SERVER
        /// [`regtypelib`]: FileFlags::REGISTER_TYPE_LIB
        /// [`sharedfile`]: FileFlags::SHARED_FILE
        /// [64-bit install mode]: https://jrsoftware.org/ishelp/topic_32vs64bitinstalls.htm
        #[doc(alias = "64bit")]
        const BITS_64 = 1 << 27;

        const EXTERNAL_SIZE_PRESET = 1 << 28;

        /// Instructs Setup to enable NTFS compression on the file (even if it didn't replace the
        /// file). If it fails to set the compression state for any reason (for example, if
        /// compression is not supported by the file system), no error message will be displayed.
        #[doc(alias = "setntfscompression")]
        const SET_NTFS_COMPRESSION = 1 << 29;

        /// Instructs Setup to disable NTFS compression on the file (even if it didn't replace the
        /// file). If it fails to set the compression state for any reason (for example, if
        /// compression is not supported by the file system), no error message will be displayed.
        #[doc(alias = "unsetntfscompression")]
        const UNSET_NTFS_COMPRESSION = 1 << 30;

        /// Install the file into the .NET Global Assembly Cache. When used in combination with
        /// [`sharedfile`], the file will only be uninstalled when the reference count reaches zero.
        ///
        /// To uninstall the file Uninstaller uses the strong assembly name specified by parameter
        /// `StrongAssemblyName`.
        ///
        /// An exception will be raised if an attempt is made to use this flag on a system with no
        /// .NET Framework present.
        ///
        /// [`sharedfile`]: FileFlags::SHARED_FILE
        #[doc(alias = "gacinstall")]
        const GAC_INSTALL = 1 << 31;

        #[doc(alias = "download")]
        const DOWNLOAD = 1 << 32;

        #[doc(alias = "extractarchive")]
        const EXTRACT_ARCHIVE = 1 << 33;

        // ~~~Obsolete options~~~

        /// File is the "README" file. Only one file in an installation can have this flag. When a
        /// file has this flag, the user will be asked if they would like to view the README file
        /// after the installation has completed. If Yes is chosen, Setup will open the file, using
        /// the default program for the file type. For this reason, the README file should always
        /// end with an extension like `.txt`, `.wri`, or `.doc`.
        ///
        /// Note that if Setup has to restart the user's computer (as a result of installing a file
        /// with the flag [`restartreplace`] or if the `AlwaysRestart [Setup]` section directive is
        /// `yes`), the user will not be given an option to view the README file.
        ///
        /// [`restartreplace`]: FileFlags::RESTART_REPLACE
        #[doc(alias = "isreadme")]
        const IS_README_FILE = 1 << 63;
    }
}

impl fmt::Display for FileFlags {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        bitflags::parser::to_writer(self, f)
    }
}
