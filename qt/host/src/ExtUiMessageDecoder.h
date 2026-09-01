#pragma once

#include <QByteArray>
#include <QObject>
#include <QVariant>

class ExtUiMessageDecoder final : public QObject
{
    Q_OBJECT

public:
    explicit ExtUiMessageDecoder(QObject *parent = nullptr);
    void decode(QByteArray payload, quint64 epoch, quint64 sequence);

private:
    struct DecodeTrace;
    static DecodeTrace beginDecodeTrace();
    static void finishDecodeTrace(DecodeTrace *trace);
    static void recordDecodedTrace(
        const QVariant &value, quint64 sequence, qsizetype payloadBytes,
        const DecodeTrace &trace);
    static void recordFailedTrace(
        const QString &message, quint64 sequence, qsizetype payloadBytes,
        const DecodeTrace &trace);

signals:
    void decoded(quint64 epoch, quint64 sequence, const QVariant &value);
    void failed(quint64 epoch, quint64 sequence, const QString &message);
};
