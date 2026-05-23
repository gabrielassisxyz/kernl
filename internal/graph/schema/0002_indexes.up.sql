CREATE INDEX idx_edges_src_label ON edges(src, label);
CREATE INDEX idx_edges_dst_label ON edges(dst, label);
CREATE INDEX idx_nodes_type ON nodes(type);
CREATE INDEX idx_node_tags_tag_id ON node_tags(tag_id);
