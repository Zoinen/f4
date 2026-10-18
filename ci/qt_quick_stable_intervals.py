"""Exact Qt 6.11.1 replacements after the order and blocked-opacity fixes.

Embedded into the Conan recipe by patch-qt-dependencies.py; no runtime helper
file is needed in exported recipes. Keep source anchors strict on Qt updates.
"""

REPLACEMENTS = (
    (r'''
namespace QSGBatchRenderer
{

#define DECLARE_DEBUG_VAR(variable) \
    static bool debug_ ## variable() \
''', r'''
namespace QSGBatchRenderer
{

struct F4PendingClip { Node *funder; int orders; };
static thread_local QHash<Renderer *, QHash<Node *, F4PendingClip>> f4PendingClips;


// A root owns its free interval. Grow through an ancestor only when that
// interval is exhausted; ordinary additions do not debit every ancestor.
static Node *f4TakeGeometryOrder(Node *root)
{
    BatchRootInfo *info = root->rootInfo();
    if (info->availableOrders > 0) {
        --info->availableOrders;
        return root;
    }
    for (Node *parent = info->parentRoot; parent; parent = parent->rootInfo()->parentRoot) {
        BatchRootInfo *parentInfo = parent->rootInfo();
        if (parentInfo->availableOrders <= 0) continue;
        const int allocation = qMin(65, parentInfo->availableOrders);
        parentInfo->availableOrders -= allocation;
        info->availableOrders = allocation - 1;
        return parent;
    }
    return nullptr;
}


#define DECLARE_DEBUG_VAR(variable) \
    static bool debug_ ## variable() \
'''),
    (r'''        e->translateOnlyToRoot = isTranslate(*gn->matrix());

        if (e->root) {
            BatchRootInfo *info = renderer->batchRootInfo(e->root);
            while (info != nullptr) {
                info->availableOrders--;
                if (info->availableOrders < 0) {
                    renderer->m_rebuild |= Renderer::BuildRenderLists;
                } else {
                    renderer->m_rebuild |= Renderer::BuildRenderListsForTaggedRoots;
                    renderer->m_taggedRoots << e->root;
                }
                if (info->parentRoot != nullptr)
                    info = renderer->batchRootInfo(info->parentRoot);
                else
                    info = nullptr;
            }
        } else {
            renderer->m_rebuild |= Renderer::FullRebuild;
''', r'''        e->translateOnlyToRoot = isTranslate(*gn->matrix());

        if (e->root) {
            if (!(renderer->m_rebuild & Renderer::BuildRenderLists)) {
                if (Node *fundingRoot = f4TakeGeometryOrder(e->root)) {
                    renderer->m_rebuild |= Renderer::BuildRenderListsForTaggedRoots;
                    renderer->m_taggedRoots << fundingRoot;
                } else {
                    renderer->m_rebuild |= Renderer::BuildRenderLists;
                }
            }
        } else {
            renderer->m_rebuild |= Renderer::FullRebuild;
'''),
    (r'''
Renderer::~Renderer()
{
    if (m_rhi) {
        // Clean up batches and buffers
        for (int i = 0; i < m_opaqueBatches.size(); ++i)
''', r'''
Renderer::~Renderer()
{
    f4PendingClips.remove(this);
    if (m_rhi) {
        // Clean up batches and buffers
        for (int i = 0; i < m_opaqueBatches.size(); ++i)
'''),
    (r'''
    } else if (node->type() == QSGNode::ClipNodeType) {
        snode->data = new ClipBatchRootInfo;
        m_rebuild |= FullRebuild;

    } else if (node->type() == QSGNode::RenderNodeType) {
        QSGRenderNode *rn = static_cast<QSGRenderNode *>(node);
''', r'''
    } else if (node->type() == QSGNode::ClipNodeType) {
        snode->data = new ClipBatchRootInfo;
        Node *fundingRoot = nullptr;
        Node *assignedAncestor = nullptr;
        for (Node *parent = shadowParent; parent; parent = parent->parent()) {
            if (parent->type() != QSGNode::ClipNodeType && !parent->isBatchRoot) continue;
            const auto *info = parent->rootInfo();
            if (!assignedAncestor && info->firstOrder >= 0) assignedAncestor = parent;
            if (!fundingRoot && info->availableOrders > 0) fundingRoot = parent;
        }
        if (!(m_rebuild & BuildRenderLists) && fundingRoot && assignedAncestor) {
            const int padding = qMin(64, fundingRoot->rootInfo()->availableOrders);
            fundingRoot->rootInfo()->availableOrders -= padding;
            snode->clipInfo()->availableOrders = padding;
            f4PendingClips[this].insert(snode, {fundingRoot, padding});
            m_taggedRoots << fundingRoot << assignedAncestor;
            m_rebuild |= BuildRenderListsForTaggedRoots;
        } else {
            m_rebuild |= FullRebuild;
        }

    } else if (node->type() == QSGNode::RenderNodeType) {
        QSGRenderNode *rn = static_cast<QSGRenderNode *>(node);
'''),
    (r'''            e->removed = true;
            m_elementsToDelete.add(e);
            e->node = nullptr;
            if (e->root) {
                BatchRootInfo *info = batchRootInfo(e->root);
                // Mirror the ancestor reservations in Updater::visitGeometryNode().
                while (info) {
                    ++info->availableOrders;
                    info = info->parentRoot ? batchRootInfo(info->parentRoot) : nullptr;
                }
            }
            if (e->batch) {
                e->batch->needsUpload = true;
                e->batch->needsPurge = true;
''', r'''            e->removed = true;
            m_elementsToDelete.add(e);
            e->node = nullptr;
            if (e->root)
                ++batchRootInfo(e->root)->availableOrders;
            if (e->batch) {
                e->batch->needsUpload = true;
                e->batch->needsPurge = true;
'''),
    (r'''        }

    } else if (node->type() == QSGNode::ClipNodeType) {
        removeBatchRootFromParent(node);
        delete node->clipInfo();
        m_rebuild |= FullRebuild;
        m_taggedRoots.remove(node);

    } else if (node->isBatchRoot) {
''', r'''        }

    } else if (node->type() == QSGNode::ClipNodeType) {
        const auto *info = node->clipInfo();
        Node *parent = info->parentRoot;
        const qint64 allocation = qint64(info->lastOrder) - info->firstOrder;
        const qint64 parentAvailable = parent ? qint64(parent->rootInfo()->availableOrders) + allocation : -1;
        bool pendingRefunded = false;
        auto pendingState = f4PendingClips.find(this);
        if (!(m_rebuild & BuildRenderLists) && info->firstOrder < 0
            && pendingState != f4PendingClips.end()) {
            const auto pending = pendingState->constFind(node);
            if (pending != pendingState->cend() && info->availableOrders == pending->orders) {
                const auto receipt = pending.value();
                auto *fundInfo = receipt.funder->rootInfo();
                const qint64 returned = qint64(fundInfo->availableOrders) + receipt.orders;
                qint64 capacity = qint64(fundInfo->lastOrder) - fundInfo->firstOrder;
                if (fundInfo->firstOrder < 0) {
                    const auto fundPending = pendingState->constFind(receipt.funder);
                    capacity = fundPending != pendingState->cend() ? fundPending->orders : -1;
                }
                if (returned >= 0 && returned <= capacity) {
                    fundInfo->availableOrders = int(returned);
                    pendingRefunded = true;
                }
            }
        }
        if (pendingRefunded) {
            // No order interval or GPU resources have been published yet.
            m_rebuild |= BuildRenderListsForTaggedRoots;
        } else if (!(m_rebuild & BuildRenderLists) && parent && info->firstOrder >= 0 && allocation >= 0
            && parent->rootInfo()->firstOrder >= 0 && parentAvailable >= 0
            && parentAvailable <= qint64(parent->rootInfo()->lastOrder) - parent->rootInfo()->firstOrder) {
            parent->rootInfo()->availableOrders = int(parentAvailable);
            // Leave resource retirement to normal render-pass cleanup. A node
            // can also be removed during shutdown, before another pass runs.
            for (auto *batches : {&m_opaqueBatches, &m_alphaBatches})
                for (int i = 0; i < batches->size(); ++i)
                    if (batches->at(i)->root == node) batches->at(i)->invalidate();
            m_rebuild |= BuildBatches;
        } else {
            m_rebuild |= FullRebuild;
        }
        if (pendingState != f4PendingClips.end()) pendingState->remove(node);
        removeBatchRootFromParent(node);
        delete node->clipInfo();
        m_taggedRoots.remove(node);

    } else if (node->isBatchRoot) {
'''),
    (r'''            m_nextRenderOrder = info->firstOrder;
            QSGNODE_TRAVERSE(node)
                    buildRenderLists(child);
            m_nextRenderOrder = info->lastOrder + 1;
        } else {
            int currentOrder = m_nextRenderOrder;
            QSGNODE_TRAVERSE(node)
                buildRenderLists(child);
            int padding = qMax((m_nextRenderOrder - currentOrder) >> 2, m_minimumOrderPadding);
            info->firstOrder = currentOrder;
            info->availableOrders = padding;
            info->lastOrder = m_nextRenderOrder + padding;
            m_nextRenderOrder = info->lastOrder;
        }
        return;
''', r'''            m_nextRenderOrder = info->firstOrder;
            QSGNODE_TRAVERSE(node)
                    buildRenderLists(child);
            m_nextRenderOrder = info->lastOrder;
        } else {
            int currentOrder = m_nextRenderOrder;
            QSGNODE_TRAVERSE(node)
                buildRenderLists(child);
            int padding = m_partialRebuild ? info->availableOrders
                : qMax((m_nextRenderOrder - currentOrder) >> 2, m_minimumOrderPadding);
            Q_ASSERT(padding >= 0);
            info->firstOrder = currentOrder;
            info->availableOrders = padding;
            info->lastOrder = m_nextRenderOrder + padding;
            if (auto pending = f4PendingClips.find(this); pending != f4PendingClips.end())
                pending->remove(shadowNode);
            m_nextRenderOrder = info->lastOrder;
        }
        return;
'''),
    (r'''    qsg_addOrphanedElements(m_tmpOpaqueElements, m_opaqueRenderList);
    qsg_addOrphanedElements(m_tmpAlphaElements, m_alphaRenderList);

    // Take a copy now, as we will be adding to this while traversing..
    QSet<Node *> roots = m_taggedRoots;
    for (QSet<Node *>::const_iterator it = roots.constBegin();
''', r'''    qsg_addOrphanedElements(m_tmpOpaqueElements, m_opaqueRenderList);
    qsg_addOrphanedElements(m_tmpAlphaElements, m_alphaRenderList);

    // Existing batches keep ownership of their buffers until the new order
    // ranges are known. Ordinary dirty/removed flags remain authoritative.
    QHash<Element *, int> previousOrders;
    QList<int> changedOrders;
    for (const auto *list : {&m_opaqueRenderList, &m_alphaRenderList}) {
        for (int j = 0; j < list->size(); ++j) {
            Element *element = list->at(j);
            if (!element) continue;
            if (element->removed) changedOrders.append(element->order);
            else previousOrders.insert(element, element->order);
        }
    }

    // Take a copy now, as we will be adding to this while traversing..
    QSet<Node *> roots = m_taggedRoots;
    for (QSet<Node *>::const_iterator it = roots.constBegin();
'''),
    (r'''        tagSubRoots(*it);
    }

    for (int i=0; i<m_opaqueBatches.size(); ++i) {
        Batch *b = m_opaqueBatches.at(i);
        if (m_taggedRoots.contains(b->root))
            invalidateAndRecycleBatch(b);

    }
    for (int i=0; i<m_alphaBatches.size(); ++i) {
        Batch *b = m_alphaBatches.at(i);
        if (m_taggedRoots.contains(b->root))
            invalidateAndRecycleBatch(b);
    }

    m_opaqueRenderList.reset();
''', r'''        tagSubRoots(*it);
    }

    struct OrderRange { int first; int second; };
    QHash<Node *, OrderRange> previousRanges;
    for (Node *root : std::as_const(m_taggedRoots)) {
        const auto *info = batchRootInfo(root);
        previousRanges.insert(root, {info->firstOrder, info->lastOrder});
    }

    m_opaqueRenderList.reset();
'''),
    (r'''    }
    m_partialRebuild = false;
    m_partialRebuildRoot = nullptr;
    m_taggedRoots.clear();
    m_nextRenderOrder = qMax(m_nextRenderOrder, maxRenderOrder);

    // Add orphaned elements back into the list and then sort it..
''', r'''    }
    m_partialRebuild = false;
    m_partialRebuildRoot = nullptr;
    m_nextRenderOrder = qMax(m_nextRenderOrder, maxRenderOrder);

    // Add orphaned elements back into the list and then sort it..
'''),
    (r'''        std::sort(&m_opaqueRenderList.first(), &m_opaqueRenderList.last() + 1, qsg_sort_element_decreasing_order);
    if (m_alphaRenderList.size())
        std::sort(&m_alphaRenderList.first(), &m_alphaRenderList.last() + 1, qsg_sort_element_increasing_order);

}

''', r'''        std::sort(&m_opaqueRenderList.first(), &m_opaqueRenderList.last() + 1, qsg_sort_element_decreasing_order);
    if (m_alphaRenderList.size())
        std::sort(&m_alphaRenderList.first(), &m_alphaRenderList.last() + 1, qsg_sort_element_increasing_order);

    for (const auto *list : {&m_opaqueRenderList, &m_alphaRenderList}) {
        for (int j = 0; j < list->size(); ++j) {
            Element *element = list->at(j);
            if (!element) continue;
            auto old = previousOrders.constFind(element);
            if (old == previousOrders.cend() || old.value() != element->order)
                changedOrders.append(element->order);
        }
    }
    std::sort(changedOrders.begin(), changedOrders.end());
    const auto retainBatch = [&](Batch *batch) {
        if (m_nextRenderOrder != maxRenderOrder || batch->isRenderNode || !batch->first
            || m_visualizer->mode() != Visualizer::VisualizeNothing)
            return false;
        const auto oldRange = previousRanges.constFind(batch->root);
        if (oldRange == previousRanges.cend()) return false;
        const auto *info = batchRootInfo(batch->root);
        if (info->firstOrder != oldRange->first || info->lastOrder != oldRange->second)
            return false;
        auto changed = std::lower_bound(changedOrders.cbegin(), changedOrders.cend(), info->firstOrder);
        if (changed != changedOrders.cend() && *changed <= info->lastOrder) return false;
        for (Element *element = batch->first; element; element = element->nextInBatch) {
            auto old = previousOrders.constFind(element);
            if (element->removed || element->root != batch->root
                || old == previousOrders.cend() || old.value() != element->order)
                return false;
        }
        return true;
    };
    for (auto *batches : {&m_opaqueBatches, &m_alphaBatches}) {
        for (int j = 0; j < batches->size(); ++j) {
            Batch *batch = batches->at(j);
            if (m_taggedRoots.contains(batch->root) && !retainBatch(batch))
                invalidateAndRecycleBatch(batch);
        }
    }
    m_taggedRoots.clear();

}

'''),
)
