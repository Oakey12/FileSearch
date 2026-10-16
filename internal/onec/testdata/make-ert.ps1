# Собирает тестовые файлы 1С:Предприятия 7.7 (составные OLE-документы) по описанию из
# исходников GComp: sample77.ert — внешняя обработка, sample77.md — конфигурация с модулями
# во вложенных хранилищах. Запуск: powershell -ExecutionPolicy Bypass -File make-ert.ps1
$ErrorActionPreference = "Stop"

Add-Type -TypeDefinition @"
using System;
using System.IO;
using System.IO.Compression;
using System.Runtime.InteropServices;
using System.Runtime.InteropServices.ComTypes;
using System.Text;

public static class Ert77 {
    [DllImport("ole32.dll")]
    static extern int StgCreateDocfile([MarshalAs(UnmanagedType.LPWStr)] string name, uint mode, uint reserved, out IStorage storage);

    [ComImport, Guid("0000000b-0000-0000-C000-000000000046"), InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
    public interface IStorage {
        void CreateStream([MarshalAs(UnmanagedType.LPWStr)] string name, uint mode, uint r1, uint r2, out IStream stream);
        void OpenStream();
        void CreateStorage([MarshalAs(UnmanagedType.LPWStr)] string name, uint mode, uint r1, uint r2, out IStorage storage);
        void OpenStorage([MarshalAs(UnmanagedType.LPWStr)] string name, IntPtr priority, uint mode, IntPtr exclude, uint reserved, out IStorage storage);
        void CopyTo(); void MoveElementTo();
        void Commit(uint flags);
    }

    const uint STGM_CREATE = 0x1000, STGM_READWRITE = 0x2, STGM_SHARE_EXCLUSIVE = 0x10;

    static void Put(IStorage st, string name, byte[] data) {
        IStream s;
        st.CreateStream(name, STGM_CREATE | STGM_READWRITE | STGM_SHARE_EXCLUSIVE, 0, 0, out s);
        s.Write(data, data.Length, IntPtr.Zero);
        Marshal.ReleaseComObject(s);
    }

    // PutAt кладёт поток по пути «Хранилище/Вложенное/Поток», создавая хранилища.
    static void PutAt(IStorage root, string path, byte[] data) {
        var parts = path.Split('/');
        var cur = root;
        var opened = new System.Collections.Generic.List<IStorage>();
        for (int i = 0; i < parts.Length - 1; i++) {
            IStorage sub;
            // Хранилище уже есть (второй модуль в том же месте) — открыть, иначе создать.
            try { cur.OpenStorage(parts[i], IntPtr.Zero, STGM_READWRITE | STGM_SHARE_EXCLUSIVE, IntPtr.Zero, 0, out sub); }
            catch (COMException) { cur.CreateStorage(parts[i], STGM_CREATE | STGM_READWRITE | STGM_SHARE_EXCLUSIVE, 0, 0, out sub); }
            opened.Add(sub);
            cur = sub;
        }
        Put(cur, parts[parts.Length - 1], data);
        for (int i = opened.Count - 1; i >= 0; i--) { opened[i].Commit(0); Marshal.ReleaseComObject(opened[i]); }
    }

    public static void MakeMD(string path, string[] modulePaths, string[] modules, string metadata) {
        var cp1251 = Encoding.GetEncoding(1251);
        IStorage st;
        int hr = StgCreateDocfile(path, STGM_CREATE | STGM_READWRITE | STGM_SHARE_EXCLUSIVE, 0, out st);
        if (hr != 0) throw new Exception("StgCreateDocfile 0x" + hr.ToString("X8"));
        PutAt(st, "Metadata/Main MetaData Stream", WithMfcLength(cp1251.GetBytes(metadata)));
        for (int i = 0; i < modulePaths.Length; i++) PutAt(st, modulePaths[i], RawDeflate(cp1251.GetBytes(modules[i])));
        st.Commit(0);
        Marshal.ReleaseComObject(st);
    }

    static byte[] RawDeflate(byte[] data) {
        var ms = new MemoryStream();
        using (var d = new DeflateStream(ms, CompressionLevel.Optimal, true)) d.Write(data, 0, data.Length);
        return ms.ToArray();
    }

    static byte[] WithMfcLength(byte[] data) {
        var ms = new MemoryStream();
        if (data.Length < 0xFF) ms.WriteByte((byte)data.Length);
        else { ms.WriteByte(0xFF); ms.Write(BitConverter.GetBytes((ushort)data.Length), 0, 2); }
        ms.Write(data, 0, data.Length);
        return ms.ToArray();
    }

    public static void Make(string path, string module, string dialog) {
        var cp1251 = Encoding.GetEncoding(1251);
        IStorage st;
        int hr = StgCreateDocfile(path, STGM_CREATE | STGM_READWRITE | STGM_SHARE_EXCLUSIVE, 0, out st);
        if (hr != 0) throw new Exception("StgCreateDocfile 0x" + hr.ToString("X8"));
        Put(st, "MD Programm text", RawDeflate(cp1251.GetBytes(module)));
        Put(st, "Dialog Stream", WithMfcLength(cp1251.GetBytes(dialog)));
        Put(st, "Container.Contents", cp1251.GetBytes("{\"Container.Contents\",{\"MetaDataDescription\",\"Main MetaData Stream\",\"Main MetaData Stream\",\"\"}}"));
        st.Commit(0);
        Marshal.ReleaseComObject(st);
    }
}
"@

$module = "Перем Итог;`r`n`r`nПроцедура Сформировать()`r`n`tИтог = 0;`r`n`tСообщить(""Отчёт сформирован"");`r`nКонецПроцедуры`r`n"
$dialog = "{""Form"",{""Обработка 7.7"",""Кнопка Сформировать""}}"
[Ert77]::Make((Join-Path $PSScriptRoot "sample77.ert"), $module, $dialog)
"Создан sample77.ert"

# Конфигурация: документ №25 «РасходнаяНакладная», справочник №31 «Номенклатура».
$metadata = '{{"MainDataContDef","1","0"},{"Documents",{"25","РасходнаяНакладная","Расходная накладная"}},{"SbCnts",{"31","Номенклатура","Номенклатура"}}}'
$paths = @(
    "GlobalData/MD Programm text",
    "TypedText/ModuleText_Number25/MD Programm text",
    "TypedText/Transact_Number25/MD Programm text",
    "Document/Document_Number25/WorkBook/MD Programm text",
    "Subconto/Subconto_Number31/WorkBook/MD Programm text"
)
$modules = @(
    "Перем глЗначение Экспорт;`r`n",
    "Процедура ПриЗаписи()`r`nКонецПроцедуры`r`n",
    "Процедура ОбработкаПроведения()`r`n`tСообщить(""Проведено"");`r`nКонецПроцедуры`r`n",
    "Процедура ПриОткрытии()`r`nКонецПроцедуры`r`n",
    "Процедура ПриВыбореЭлемента()`r`nКонецПроцедуры`r`n"
)
[Ert77]::MakeMD((Join-Path $PSScriptRoot "sample77.md"), $paths, $modules, $metadata)
"Создан sample77.md"
