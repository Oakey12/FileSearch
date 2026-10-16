# Собирает тестовый файл sample77.ert в формате 1С:Предприятия 7.7 (составной OLE-документ)
# по описанию из исходников GComp. Запуск: powershell -ExecutionPolicy Bypass -File make-ert.ps1
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
        void OpenStream(); void CreateStorage(); void OpenStorage(); void CopyTo(); void MoveElementTo();
        void Commit(uint flags);
    }

    const uint STGM_CREATE = 0x1000, STGM_READWRITE = 0x2, STGM_SHARE_EXCLUSIVE = 0x10;

    static void Put(IStorage st, string name, byte[] data) {
        IStream s;
        st.CreateStream(name, STGM_CREATE | STGM_READWRITE | STGM_SHARE_EXCLUSIVE, 0, 0, out s);
        s.Write(data, data.Length, IntPtr.Zero);
        Marshal.ReleaseComObject(s);
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
