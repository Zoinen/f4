package id3editor

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/id3-go"
	v1 "github.com/unxed/id3-go/v1"
	v2 "github.com/unxed/id3-go/v2"
	"github.com/unxed/vtui"
)

type ID3EditorPlugin struct {
	api vfs.HostAPI
}

func (p *ID3EditorPlugin) Init(api vfs.HostAPI) error {
	p.api = api
	api.RegisterPluginMenuItem(vtui.Msg("ID3Editor.Menu"), func(app vfs.App) {
		p.handleEdit(app)
	})
	return nil
}

func (p *ID3EditorPlugin) Close() error {
	return nil
}

func (p *ID3EditorPlugin) GetName() string {
	return "ID3 Tag Editor"
}

func (p *ID3EditorPlugin) handleEdit(app vfs.App) {
	activeVFS := app.GetActivePanelVFS()
	if activeVFS == nil {
		vtui.ShowMessage(" Error ", vtui.Msg("ID3Editor.LocalOnly"), []string{"&Ok"})
		return
	}

	if _, isLocal := activeVFS.(*vfs.OSVFS); !isLocal {
		vtui.ShowMessage(" Error ", vtui.Msg("ID3Editor.LocalOnly"), []string{"&Ok"})
		return
	}

	names := app.GetSelectedNames()
	if len(names) == 0 {
		// Neutral hint ("select a file first"), not an alarm — a
		// non-whitelist title keeps this on the info palette. See #379.
		vtui.ShowMessage(" ID3 Editor ", vtui.Msg("ID3Editor.SelectFile"), []string{"&Ok"})
		return
	}

	name := names[0]
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".mp3" {
		// Same rationale as the SelectFile hint above.
		vtui.ShowMessage(" ID3 Editor ", vtui.Msg("ID3Editor.OnlyMP3"), []string{"&Ok"})
		return
	}

	fullPath, err := activeVFS.Abs(name)
	if err != nil {
		vtui.ShowMessage(" Error ", fmt.Sprintf(vtui.Msg("ID3Editor.OpenError"), err), []string{"&Ok"})
		return
	}

	p.showEditorDialog(app, fullPath)
}

func (p *ID3EditorPlugin) showEditorDialog(app vfs.App, fullPath string) {
	file, err := id3.Open(fullPath)
	if err != nil {
		app.Message(" Error ", fmt.Sprintf(vtui.Msg("ID3Editor.OpenError"), err), []string{"&Ok"})
		return
	}

	width, height := 66, 17
	dlg := vtui.NewCenteredDialog(width, height, vtui.Msg("ID3Editor.Title"))
	dlg.ShowClose = true

	cleanStr := func(s string) string {
		return strings.TrimSpace(strings.TrimRight(s, "\x00"))
	}

	titleText := cleanStr(file.Title())
	artistText := cleanStr(file.Artist())
	albumText := cleanStr(file.Album())
	yearText := cleanStr(file.Year())
	genreText := cleanStr(file.Genre())

	commentText := ""
	if comments := file.Comments(); len(comments) > 0 {
		commentText = cleanStr(comments[0])
	}

	editTitle := vtui.NewEdit(0, 0, 44, titleText)
	editArtist := vtui.NewEdit(0, 0, 44, artistText)
	editAlbum := vtui.NewEdit(0, 0, 44, albumText)
	editYear := vtui.NewEdit(0, 0, 10, yearText)
	editGenre := vtui.NewEdit(0, 0, 20, genreText)
	editComment := vtui.NewEdit(0, 0, 44, commentText)

	padLabel := func(s string) string {
		for len([]rune(s)) < 10 {
			s += " "
		}
		return s
	}

	lblTitle := vtui.NewLabel(0, 0, padLabel(vtui.Msg("ID3Editor.FieldTitle")), editTitle)
	lblArtist := vtui.NewLabel(0, 0, padLabel(vtui.Msg("ID3Editor.FieldArtist")), editArtist)
	lblAlbum := vtui.NewLabel(0, 0, padLabel(vtui.Msg("ID3Editor.FieldAlbum")), editAlbum)
	lblYear := vtui.NewLabel(0, 0, padLabel(vtui.Msg("ID3Editor.FieldYear")), editYear)
	lblGenre := vtui.NewLabel(0, 0, padLabel(vtui.Msg("ID3Editor.FieldGenre")), editGenre)
	lblComment := vtui.NewLabel(0, 0, padLabel(vtui.Msg("ID3Editor.FieldComment")), editComment)

	btnSave := vtui.NewButton(0, 0, vtui.Msg("vtui.Save"))
	btnSave.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, vtui.Msg("vtui.Cancel"))

	dlg.AddItem(lblTitle)
	dlg.AddItem(editTitle)
	dlg.AddItem(lblArtist)
	dlg.AddItem(editArtist)
	dlg.AddItem(lblAlbum)
	dlg.AddItem(editAlbum)
	dlg.AddItem(lblYear)
	dlg.AddItem(editYear)
	dlg.AddItem(lblGenre)
	dlg.AddItem(editGenre)
	dlg.AddItem(lblComment)
	dlg.AddItem(editComment)
	dlg.AddItem(btnSave)
	dlg.AddItem(btnCancel)

	versionText := file.Version()
	lblVersion := vtui.NewLabel(0, 0, padLabel(vtui.Msg("ID3Editor.FieldVersion")), nil)
	valVersion := vtui.NewLabel(0, 0, versionText, nil)
	dlg.AddItem(lblVersion)
	dlg.AddItem(valVersion)
	valVersion.SetText(versionText)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)

	addFormRow := func(lbl vtui.UIElement, edit vtui.UIElement, topMargin int) {
		row := vtui.NewHBoxLayout(0, 0, width-4, 1)
		row.Add(lbl, vtui.Margins{Right: 1}, vtui.AlignLeft)
		row.Add(edit, vtui.Margins{}, vtui.AlignFill)
		vbox.Add(row, vtui.Margins{Top: topMargin}, vtui.AlignFill)
	}

	addFormRow(lblTitle, editTitle, 0)
	addFormRow(lblArtist, editArtist, 1)
	addFormRow(lblAlbum, editAlbum, 1)

	rowYearGenre := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowYearGenre.Add(lblYear, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowYearGenre.Add(editYear, vtui.Margins{Right: 2}, vtui.AlignLeft)
	rowYearGenre.Add(lblGenre, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowYearGenre.Add(editGenre, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(rowYearGenre, vtui.Margins{Top: 1}, vtui.AlignFill)

	addFormRow(lblComment, editComment, 1)

	rowVersion := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowVersion.Add(lblVersion, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowVersion.Add(valVersion, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(rowVersion, vtui.Margins{Top: 1}, vtui.AlignFill)

	rowBtns := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowBtns.HorizontalAlign = vtui.AlignCenter
	rowBtns.Spacing = 2
	rowBtns.Add(btnSave, vtui.Margins{}, vtui.AlignTop)
	rowBtns.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(rowBtns, vtui.Margins{Top: 1}, vtui.AlignFill)

	vbox.Apply()

	closed := false
	safeClose := func() {
		if !closed {
			_ = file.Close()
			closed = true
		}
	}

	btnCancel.OnClick = func() {
		safeClose()
		dlg.Close()
	}

	btnSave.OnClick = func() {
		file.SetTitle(editTitle.GetText())
		file.SetArtist(editArtist.GetText())
		file.SetAlbum(editAlbum.GetText())
		file.SetYear(editYear.GetText())
		file.SetGenre(editGenre.GetText())
		setComment(file.Tagger, editComment.GetText())

		err := file.Close()
		closed = true
		if err != nil {
			app.Message(" Error ", fmt.Sprintf(vtui.Msg("ID3Editor.SaveError"), err), []string{"&Ok"})
			return
		}

		dlg.Close()
		app.RefreshAll()
	}

	dlg.OnResult = func(code int) {
		safeClose()
	}

	vtui.FrameManager.Push(dlg)
}

func setComment(tagger id3.Tagger, comment string) {
	switch t := tagger.(type) {
	case *v1.Tag:
		t.SetComment(comment)
	case *v2.Tag:
		if strings.HasPrefix(t.Version(), "2.2.") {
			t.DeleteFrames("COM")
			ft := v2.V22FrameTypeMap["COM"]
			f := v2.NewUnsynchTextFrame(ft, "", comment)
			_ = f.SetEncoding("UTF-8")
			t.AddFrames(f)
		} else {
			t.DeleteFrames("COMM")
			ft := v2.V23FrameTypeMap["COMM"]
			f := v2.NewUnsynchTextFrame(ft, "", comment)
			_ = f.SetEncoding("UTF-8")
			t.AddFrames(f)
		}
	}
}
